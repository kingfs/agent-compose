package llm

import (
	appconfig "agent-compose/pkg/config"
	"context"
	"fmt"
	"net/url"
	"sort"
	"strings"
)

func bootstrapDefaultLLMConfig(ctx context.Context, config *appconfig.Config, store *Store, requestedModel string) error {
	if hasConfiguredLLMProviderForFamily(ctx, store, llmProviderFamilyOpenAI) {
		return nil
	}
	return ensureDefaultOpenAIEnvProvider(ctx, config, store, requestedModel)
}

// ensureOpenAIEnvProvider upserts an OpenAI-family provider from a resolved env
// lookup. It returns the provider id (empty when nothing was configured).
func ensureOpenAIEnvProvider(ctx context.Context, store *Store, lookup llmEnvProviderLookup, providerID, name, scope, requestedModel string, defaultModel bool) (string, error) {
	endpoint := firstNonEmpty(lookup("LLM_API_ENDPOINT"), "https://api.openai.com")
	if looksLikeAnthropicMessagesEndpoint(endpoint) {
		return "", nil
	}
	protocol := normalizeLLMWireAPI(lookup("LLM_API_PROTOCOL"))
	apiKey := lookup("LLM_API_KEY", "OPENAI_API_KEY")
	model := strings.TrimSpace(firstNonEmpty(requestedModel, lookup("LLM_MODEL")))
	if providerID == "" || model == "" {
		return "", nil
	}
	return providerID, store.UpsertDefaultLLMConfig(ctx, LLMProvider{
		ID:             providerID,
		Name:           name,
		ProviderType:   llmProviderFamilyOpenAI,
		DefaultWireAPI: protocol,
		BaseURL:        endpoint,
		APIKey:         apiKey,
		AuthHeader:     "Authorization",
		AuthScheme:     "Bearer",
		HeadersJSON:    "{}",
		Weight:         10,
		Enabled:        true,
		Scope:          scope,
	}, LLMModel{ID: model, Name: model, DefaultModel: defaultModel, Enabled: true, Scope: scope})
}

// ensureAnthropicEnvProvider upserts an Anthropic-family provider from a resolved
// env lookup. It returns the provider id (empty when nothing was configured).
func ensureAnthropicEnvProvider(ctx context.Context, store *Store, lookup llmEnvProviderLookup, authHeader, authScheme, providerID, name, scope, requestedModel string, defaultModel bool) (string, error) {
	anthropicEndpoint := lookup("ANTHROPIC_BASE_URL", "ANTHROPIC_API_ENDPOINT")
	genericEndpoint := lookup("LLM_API_ENDPOINT")
	anthropicKey := lookup("ANTHROPIC_API_KEY", "ANTHROPIC_AUTH_TOKEN")
	anthropicModel := lookup("ANTHROPIC_MODEL", "CLAUDE_MODEL")
	genericModel := lookup("LLM_MODEL")
	useGenericEndpoint := anthropicEndpoint == "" && looksLikeAnthropicMessagesEndpoint(genericEndpoint)
	if useGenericEndpoint {
		anthropicEndpoint = genericEndpoint
	}
	if genericModel != "" && (useGenericEndpoint || anthropicEndpoint != "" || anthropicKey != "" || anthropicModel != "") {
		anthropicModel = firstNonEmpty(anthropicModel, genericModel)
	}
	if anthropicEndpoint == "" && strings.TrimSpace(anthropicKey) == "" && strings.TrimSpace(anthropicModel) == "" {
		return "", nil
	}
	endpoint := firstNonEmpty(anthropicEndpoint, "https://api.anthropic.com")
	apiKey := firstNonEmpty(anthropicKey, lookup("LLM_API_KEY"))
	model := strings.TrimSpace(firstNonEmpty(requestedModel, anthropicModel))
	if providerID == "" || model == "" {
		return "", nil
	}
	return providerID, store.UpsertDefaultLLMConfig(ctx, LLMProvider{
		ID:             providerID,
		Name:           name,
		ProviderType:   llmProviderFamilyAnthropic,
		DefaultWireAPI: llmAPIProtocolMessages,
		BaseURL:        endpoint,
		APIKey:         apiKey,
		AuthHeader:     authHeader,
		AuthScheme:     authScheme,
		HeadersJSON:    `{"anthropic-version":"2023-06-01"}`,
		Weight:         10,
		Enabled:        true,
		Scope:          scope,
	}, LLMModel{ID: model, Name: model, DefaultModel: defaultModel, Enabled: true, Scope: scope})
}

func ensureDefaultOpenAIEnvProvider(ctx context.Context, config *appconfig.Config, store *Store, requestedModel string) error {
	_, err := ensureOpenAIEnvProvider(ctx, store, defaultLLMEnvProviderLookup(ctx, config, store), llmProviderIDDefaultOpenAI, "default", llmProviderScopeEnvDefault, requestedModel, true)
	return err
}

func resolveLLMTarget(ctx context.Context, config *appconfig.Config, store *Store, requestedModel string) (LLMResolvedTarget, error) {
	return resolveLLMTargetForProviderFamily(ctx, config, store, llmProviderFamilyOpenAI, requestedModel)
}

func resolveRuntimeLLMTarget(ctx context.Context, config *appconfig.Config, store *Store, requestedModel, providerID string) (LLMResolvedTarget, error) {
	return resolveRuntimeLLMTargetWithEnv(ctx, config, store, "", "", requestedModel, providerID, nil)
}

func resolveRuntimeLLMTargetWithEnv(ctx context.Context, config *appconfig.Config, store *Store, sessionID, preferredProviderFamily, requestedModel, providerID string, envItems []EnvVar) (LLMResolvedTarget, error) {
	sessionID = strings.TrimSpace(sessionID)
	preferredProviderFamily = normalizeOptionalLLMProviderType(preferredProviderFamily)
	requestedModel = strings.TrimSpace(requestedModel)
	providerID = strings.TrimSpace(providerID)
	hasSessionEnvProvider := sessionID != "" && hasSessionEnvProviderInput(envItems)
	sessionProviderID := ""
	// Reuse an already-persisted session-env provider when this session can no
	// longer supply a key from env. The raw key env (Session.ProviderEnvItems) is
	// intentionally not persisted, so after a stop/resume the only durable home
	// for a session-scoped credential is the llm_provider row written at creation.
	// Pin its provider id here so resolution selects it (session-env providers are
	// otherwise skipped without an explicit id) and does not clobber its key with
	// the now-empty env. Only when the env still has no key for the family — an env
	// that carries a (possibly rotated) key must keep re-bootstrapping it.
	if providerID == "" && sessionID != "" && preferredProviderFamily != "" && !envHasProviderKeyForFamily(envItems, preferredProviderFamily) {
		if candidate := sessionEnvProviderID(sessionID, preferredProviderFamily); hasEnabledLLMProviderID(ctx, store, candidate) {
			providerID = candidate
		}
	}
	// Skip the env/default bootstrap entirely when the request already names a
	// provider that exists. The facade hot path always passes a concrete
	// provider id from the token scope, so this avoids a redundant pair of
	// idempotent provider upserts on every LLM request.
	bootstrapProviders := (providerID == "" || !isSessionEnvProviderID(providerID)) && !hasEnabledLLMProviderID(ctx, store, providerID)
	if bootstrapProviders && !hasConfiguredLLMProviderForFamily(ctx, store, llmProviderFamilyOpenAI) {
		openAIModel := firstNonEmpty(requestedModel, lookupEnvItemValue(envItems, "LLM_MODEL"))
		if sessionID != "" && hasOpenAIEnvProviderInput(envItems) {
			id, err := ensureSessionOpenAIEnvProvider(ctx, store, sessionID, openAIModel, envItems)
			if err != nil {
				return LLMResolvedTarget{}, err
			}
			sessionProviderID = chooseSessionEnvProviderID(sessionProviderID, id, llmProviderFamilyOpenAI, preferredProviderFamily)
		} else if !hasSessionEnvProvider {
			if err := ensureDefaultOpenAIEnvProvider(ctx, config, store, openAIModel); err != nil {
				return LLMResolvedTarget{}, err
			}
		}
	}
	if bootstrapProviders && !hasConfiguredLLMProviderForFamily(ctx, store, llmProviderFamilyAnthropic) {
		anthropicModel := firstNonEmpty(requestedModel, sessionAnthropicEnvModel(envItems))
		if sessionID != "" && hasAnthropicEnvProviderInput(envItems) {
			id, err := ensureSessionAnthropicEnvProvider(ctx, store, sessionID, anthropicModel, envItems)
			if err != nil {
				return LLMResolvedTarget{}, err
			}
			sessionProviderID = chooseSessionEnvProviderID(sessionProviderID, id, llmProviderFamilyAnthropic, preferredProviderFamily)
		} else if !hasSessionEnvProvider {
			if err := ensureDefaultAnthropicEnvProvider(ctx, config, store, anthropicModel); err != nil {
				return LLMResolvedTarget{}, err
			}
		}
	}
	providerID = firstNonEmpty(providerID, sessionProviderID)
	models, err := store.ListEnabledLLMModels(ctx)
	if err != nil {
		return LLMResolvedTarget{}, err
	}
	if len(models) == 0 {
		return LLMResolvedTarget{}, fmt.Errorf("llm model is required")
	}
	providers, err := store.ListEnabledLLMProviders(ctx)
	if err != nil {
		return LLMResolvedTarget{}, err
	}
	if len(providers) == 0 {
		return LLMResolvedTarget{}, fmt.Errorf("llm provider is not configured")
	}
	model, provider, wireAPI, ok, err := selectLLMModelAndProvider(ctx, store, models, providers, requestedModel, preferredProviderFamily, providerID)
	if err != nil {
		return LLMResolvedTarget{}, err
	}
	if !ok {
		if requestedModel != "" && providerID != "" {
			return LLMResolvedTarget{}, fmt.Errorf("llm model %q is not configured for provider %q", requestedModel, providerID)
		}
		if requestedModel != "" {
			return LLMResolvedTarget{}, fmt.Errorf("llm model %q is not configured", requestedModel)
		}
		if providerID != "" {
			return LLMResolvedTarget{}, fmt.Errorf("llm provider %q is not configured", providerID)
		}
		return LLMResolvedTarget{}, fmt.Errorf("llm provider is not configured")
	}
	endpoint := llmEndpointForProvider(provider, wireAPI)
	headers, err := providerForwardHeaders(provider)
	if err != nil {
		return LLMResolvedTarget{}, err
	}
	return LLMResolvedTarget{Provider: provider, Model: model, WireAPI: wireAPI, Endpoint: endpoint, Headers: headers}, nil
}

func bootstrapAnthropicLLMConfig(ctx context.Context, config *appconfig.Config, store *Store, requestedModel string) error {
	if hasConfiguredLLMProviderForFamily(ctx, store, llmProviderFamilyAnthropic) {
		return nil
	}
	return ensureDefaultAnthropicEnvProvider(ctx, config, store, requestedModel)
}

func ensureDefaultAnthropicEnvProvider(ctx context.Context, config *appconfig.Config, store *Store, requestedModel string) error {
	lookup := defaultLLMEnvProviderLookup(ctx, config, store)
	authHeader, authScheme := anthropicProviderAuthFromLookup(lookup)
	_, err := ensureAnthropicEnvProvider(ctx, store, lookup, authHeader, authScheme, llmProviderIDDefaultAnthropic, "anthropic", llmProviderScopeEnvDefault, requestedModel, false)
	return err
}

func looksLikeAnthropicMessagesEndpoint(endpoint string) bool {
	endpoint = strings.TrimRight(strings.TrimSpace(endpoint), "/")
	if endpoint == "" {
		return false
	}
	parsed, err := url.Parse(endpoint)
	if err != nil {
		return strings.HasSuffix(endpoint, "/messages")
	}
	return strings.HasSuffix(strings.TrimRight(parsed.Path, "/"), "/messages")
}

// anthropicProviderAuthFromLookup chooses the Anthropic auth header from the same
// env source(s) the provider's API key is resolved from, so a provider never
// mixes a key from one scope with a header decided by another scope.
func anthropicProviderAuthFromLookup(lookup llmEnvProviderLookup) (string, string) {
	if strings.TrimSpace(lookup("ANTHROPIC_API_KEY")) != "" {
		return "x-api-key", ""
	}
	if strings.TrimSpace(lookup("ANTHROPIC_AUTH_TOKEN")) != "" {
		return "Authorization", "Bearer"
	}
	return "x-api-key", ""
}

func ensureSessionOpenAIEnvProvider(ctx context.Context, store *Store, sessionID, requestedModel string, envItems []EnvVar) (string, error) {
	providerID := sessionEnvProviderID(sessionID, llmProviderFamilyOpenAI)
	return ensureOpenAIEnvProvider(ctx, store, sessionLLMEnvProviderLookup(envItems), providerID, providerID, llmProviderScopeSessionEnv, requestedModel, false)
}

func ensureSessionAnthropicEnvProvider(ctx context.Context, store *Store, sessionID, requestedModel string, envItems []EnvVar) (string, error) {
	providerID := sessionEnvProviderID(sessionID, llmProviderFamilyAnthropic)
	lookup := sessionLLMEnvProviderLookup(envItems)
	authHeader, authScheme := anthropicProviderAuthFromLookup(lookup)
	return ensureAnthropicEnvProvider(ctx, store, lookup, authHeader, authScheme, providerID, providerID, llmProviderScopeSessionEnv, requestedModel, false)
}

func hasEnabledLLMProviderID(ctx context.Context, store *Store, providerID string) bool {
	providerID = strings.TrimSpace(providerID)
	if store == nil || providerID == "" {
		return false
	}
	providers, err := store.ListEnabledLLMProviders(ctx)
	if err != nil {
		return false
	}
	for _, provider := range providers {
		if provider.ID == providerID {
			return true
		}
	}
	return false
}

func hasConfiguredLLMProviderForFamily(ctx context.Context, store *Store, providerFamily string) bool {
	if store == nil {
		return false
	}
	providers, err := store.ListEnabledLLMProviders(ctx)
	if err != nil {
		return false
	}
	for _, provider := range providers {
		if normalizeLLMProviderType(provider.ProviderType) != normalizeLLMProviderType(providerFamily) {
			continue
		}
		if llmProviderScopeIsConfigured(provider.Scope) {
			return true
		}
	}
	return false
}

func llmProviderScopeIsConfigured(scope string) bool {
	switch strings.TrimSpace(scope) {
	case llmProviderScopeEnvDefault, llmProviderScopeSessionEnv:
		return false
	default:
		return true
	}
}

func sessionEnvProviderID(sessionID, providerFamily string) string {
	sessionID = strings.TrimSpace(sessionID)
	providerFamily = normalizeOptionalLLMProviderType(providerFamily)
	if sessionID == "" || providerFamily == "" {
		return ""
	}
	return "session-env:" + sessionID + ":" + providerFamily
}

func isSessionEnvProviderID(providerID string) bool {
	return strings.HasPrefix(strings.TrimSpace(providerID), "session-env:")
}

func chooseSessionEnvProviderID(current, next, nextFamily, preferredFamily string) string {
	next = strings.TrimSpace(next)
	if next == "" {
		return current
	}
	if strings.TrimSpace(current) == "" {
		return next
	}
	preferredFamily = normalizeOptionalLLMProviderType(preferredFamily)
	if preferredFamily != "" && normalizeLLMProviderType(nextFamily) == preferredFamily {
		return next
	}
	return current
}

func resolveLLMTargetForProviderFamily(ctx context.Context, config *appconfig.Config, store *Store, providerFamily, requestedModel string) (LLMResolvedTarget, error) {
	if strings.TrimSpace(providerFamily) != "" {
		providerFamily = normalizeLLMProviderType(providerFamily)
	}
	switch providerFamily {
	case llmProviderFamilyAnthropic:
		if err := bootstrapAnthropicLLMConfig(ctx, config, store, strings.TrimSpace(requestedModel)); err != nil {
			return LLMResolvedTarget{}, err
		}
	default:
		if err := bootstrapDefaultLLMConfig(ctx, config, store, strings.TrimSpace(requestedModel)); err != nil {
			return LLMResolvedTarget{}, err
		}
	}
	models, err := store.ListEnabledLLMModels(ctx)
	if err != nil {
		return LLMResolvedTarget{}, err
	}
	if len(models) == 0 {
		return LLMResolvedTarget{}, fmt.Errorf("llm model is required")
	}
	providers, err := store.ListEnabledLLMProviders(ctx)
	if err != nil {
		return LLMResolvedTarget{}, err
	}
	if len(providers) == 0 {
		return LLMResolvedTarget{}, fmt.Errorf("llm provider is not configured")
	}
	model, provider, wireAPI, ok, err := selectLLMModelAndProvider(ctx, store, models, providers, requestedModel, providerFamily, "")
	if err != nil {
		return LLMResolvedTarget{}, err
	}
	if !ok {
		if strings.TrimSpace(requestedModel) != "" {
			return LLMResolvedTarget{}, fmt.Errorf("llm model %q is not configured for provider family %q", strings.TrimSpace(requestedModel), providerFamily)
		}
		return LLMResolvedTarget{}, fmt.Errorf("llm provider is not configured for provider family %q", providerFamily)
	}
	endpoint := llmEndpointForProvider(provider, wireAPI)
	headers, err := providerForwardHeaders(provider)
	if err != nil {
		return LLMResolvedTarget{}, err
	}
	return LLMResolvedTarget{Provider: provider, Model: model, WireAPI: wireAPI, Endpoint: endpoint, Headers: headers}, nil
}

func selectLLMModel(models []LLMModel, requested string) LLMModel {
	requested = strings.TrimSpace(requested)
	for _, model := range models {
		if requested != "" && (model.ID == requested || model.Name == requested) {
			return model
		}
	}
	if requested != "" {
		return LLMModel{}
	}
	for _, model := range models {
		if model.DefaultModel {
			return model
		}
	}
	return models[0]
}

func selectLLMModelAndProvider(ctx context.Context, store *Store, models []LLMModel, providers []LLMProvider, requestedModel, providerFamily, providerID string) (LLMModel, LLMProvider, string, bool, error) {
	if strings.TrimSpace(requestedModel) != "" {
		requested := selectLLMModel(models, requestedModel)
		if strings.TrimSpace(requested.ID) == "" {
			return LLMModel{}, LLMProvider{}, "", false, nil
		}
		provider, wireAPI, ok, err := selectLLMProviderForModel(ctx, store, providers, requested.ID, providerFamily, providerID)
		return requested, provider, wireAPI, ok, err
	}
	ordered := append([]LLMModel(nil), models...)
	sort.SliceStable(ordered, func(i, j int) bool {
		if ordered[i].DefaultModel != ordered[j].DefaultModel {
			return ordered[i].DefaultModel
		}
		return ordered[i].ID < ordered[j].ID
	})
	for _, model := range ordered {
		provider, wireAPI, ok, err := selectLLMProviderForModel(ctx, store, providers, model.ID, providerFamily, providerID)
		if err != nil {
			return LLMModel{}, LLMProvider{}, "", false, err
		}
		if ok {
			return model, provider, wireAPI, true, nil
		}
	}
	return LLMModel{}, LLMProvider{}, "", false, nil
}

func selectLLMProviderForModel(ctx context.Context, store *Store, providers []LLMProvider, modelID, providerFamily, providerID string) (LLMProvider, string, bool, error) {
	type candidate struct {
		provider LLMProvider
		wireAPI  string
		priority int
	}
	if strings.TrimSpace(providerFamily) != "" {
		providerFamily = normalizeLLMProviderType(providerFamily)
	}
	providerID = strings.TrimSpace(providerID)
	var candidates []candidate
	for _, provider := range providers {
		if providerID == "" && providerFamily != "" && normalizeLLMProviderType(provider.ProviderType) != providerFamily {
			continue
		}
		if providerID != "" && provider.ID != providerID {
			continue
		}
		if providerID == "" && strings.TrimSpace(provider.Scope) == llmProviderScopeSessionEnv {
			continue
		}
		wireAPI, ok, err := store.LLMProviderModelWireAPI(ctx, provider.ID, modelID)
		if err != nil {
			return LLMProvider{}, "", false, err
		}
		if !ok {
			continue
		}
		candidates = append(candidates, candidate{provider: provider, wireAPI: firstNonEmpty(wireAPI, normalizeLLMWireAPI(provider.DefaultWireAPI)), priority: llmProviderSelectionPriority(provider.Scope)})
	}
	if len(candidates) == 0 {
		return LLMProvider{}, "", false, nil
	}
	sort.Slice(candidates, func(i, j int) bool {
		if candidates[i].priority != candidates[j].priority {
			return candidates[i].priority < candidates[j].priority
		}
		if candidates[i].provider.Weight == candidates[j].provider.Weight {
			return candidates[i].provider.ID < candidates[j].provider.ID
		}
		return candidates[i].provider.Weight < candidates[j].provider.Weight
	})
	return candidates[0].provider, candidates[0].wireAPI, true, nil
}

func llmProviderSelectionPriority(scope string) int {
	switch strings.TrimSpace(scope) {
	case llmProviderScopeSessionEnv:
		return 2
	case llmProviderScopeEnvDefault:
		return 1
	default:
		return 0
	}
}
