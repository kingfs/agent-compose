package llm

import (
	appconfig "agent-compose/pkg/config"
	driverpkg "agent-compose/pkg/driver"
	"context"
	"os"
	"strings"
)

// llmEnvProviderLookup resolves an environment value for LLM provider bootstrap.
// It accepts candidate keys and returns the first non-empty value scanning
// sources in priority order (source-major): an earlier source wins across all
// candidate keys before a later source is consulted. This preserves the exact
// precedence the bootstrap paths relied on when they used nested firstNonEmpty.
type llmEnvProviderLookup func(keys ...string) string

// defaultLLMEnvProviderLookup reads from global env, then the process env, then
// daemon config. Used by the env_default bootstrap providers.
func defaultLLMEnvProviderLookup(ctx context.Context, config *appconfig.Config, store *Store) llmEnvProviderLookup {
	return func(keys ...string) string {
		for _, key := range keys {
			if v := lookupEnvValue(ctx, store, key); strings.TrimSpace(v) != "" {
				return v
			}
		}
		for _, key := range keys {
			if v := os.Getenv(key); strings.TrimSpace(v) != "" {
				return v
			}
		}
		for _, key := range keys {
			if v := configLLMEnvValue(config, key); strings.TrimSpace(v) != "" {
				return v
			}
		}
		return ""
	}
}

// sessionLLMEnvProviderLookup reads only from per-session env items. Used by the
// session_env bootstrap providers.
func sessionLLMEnvProviderLookup(envItems []EnvVar) llmEnvProviderLookup {
	return func(keys ...string) string {
		for _, key := range keys {
			if v := lookupEnvItemValue(envItems, key); strings.TrimSpace(v) != "" {
				return v
			}
		}
		return ""
	}
}

func configLLMEnvValue(config *appconfig.Config, key string) string {
	if config == nil {
		return ""
	}
	switch strings.ToUpper(strings.TrimSpace(key)) {
	case "LLM_API_ENDPOINT":
		return config.LLMAPIEndpoint
	case "LLM_API_PROTOCOL":
		return config.LLMAPIProtocol
	case "LLM_API_KEY":
		return config.LLMAPIKey
	case "LLM_MODEL":
		return config.LLMModel
	default:
		return ""
	}
}

// envHasProviderKeyForFamily reports whether the given env carries a usable
// provider credential for the family. Unlike hasOpenAIEnvProviderInput it checks
// for an actual key (not just an endpoint), so callers can distinguish "the env
// can (re)bootstrap a provider with a fresh key" from "the env has no key and we
// should reuse the already-persisted session-env provider".
func envHasProviderKeyForFamily(envItems []EnvVar, providerFamily string) bool {
	switch normalizeLLMProviderType(providerFamily) {
	case llmProviderFamilyAnthropic:
		return strings.TrimSpace(firstNonEmpty(
			lookupEnvItemValue(envItems, "ANTHROPIC_API_KEY"),
			lookupEnvItemValue(envItems, "ANTHROPIC_AUTH_TOKEN"),
			lookupEnvItemValue(envItems, "LLM_API_KEY"),
		)) != ""
	case llmProviderFamilyOpenAI:
		return strings.TrimSpace(firstNonEmpty(
			lookupEnvItemValue(envItems, "LLM_API_KEY"),
			lookupEnvItemValue(envItems, "OPENAI_API_KEY"),
		)) != ""
	default:
		return false
	}
}

func hasOpenAIEnvProviderInput(envItems []EnvVar) bool {
	endpoint := lookupEnvItemValue(envItems, "LLM_API_ENDPOINT")
	if looksLikeAnthropicMessagesEndpoint(endpoint) {
		return false
	}
	return strings.TrimSpace(firstNonEmpty(
		endpoint,
		lookupEnvItemValue(envItems, "LLM_API_KEY"),
		lookupEnvItemValue(envItems, "OPENAI_API_KEY"),
	)) != ""
}

func hasAnthropicEnvProviderInput(envItems []EnvVar) bool {
	return strings.TrimSpace(firstNonEmpty(
		lookupEnvItemValue(envItems, "ANTHROPIC_BASE_URL"),
		lookupEnvItemValue(envItems, "ANTHROPIC_API_ENDPOINT"),
		lookupEnvItemValue(envItems, "ANTHROPIC_API_KEY"),
		lookupEnvItemValue(envItems, "ANTHROPIC_AUTH_TOKEN"),
	)) != "" || looksLikeAnthropicMessagesEndpoint(lookupEnvItemValue(envItems, "LLM_API_ENDPOINT"))
}

func hasSessionEnvProviderInput(envItems []EnvVar) bool {
	return hasOpenAIEnvProviderInput(envItems) || hasAnthropicEnvProviderInput(envItems)
}

func sessionAnthropicEnvModel(envItems []EnvVar) string {
	genericModel := lookupEnvItemValue(envItems, "LLM_MODEL")
	return firstNonEmpty(
		lookupEnvItemValue(envItems, "ANTHROPIC_MODEL"),
		lookupEnvItemValue(envItems, "CLAUDE_MODEL"),
		genericModel,
	)
}

func lookupEnvValue(ctx context.Context, store *Store, key string) string {
	if store == nil {
		return ""
	}
	items, err := store.ListGlobalEnv(ctx)
	if err != nil {
		return ""
	}
	for _, item := range items {
		if item.Name == key {
			return item.Value
		}
	}
	return ""
}

func lookupEnvItemValue(items []EnvVar, key string) string {
	key = strings.TrimSpace(key)
	if key == "" {
		return ""
	}
	for _, item := range normalizeEnvItems(items) {
		if strings.EqualFold(strings.TrimSpace(item.Name), key) {
			return strings.TrimSpace(item.Value)
		}
	}
	return ""
}

func llmProviderKeyName(name string) bool {
	return driverpkg.LLMProviderKeyName(name)
}

func filterPersistedRuntimeEnv(items []EnvVar) []EnvVar {
	result := make([]EnvVar, 0, len(items))
	for _, item := range normalizeEnvItems(items) {
		if llmProviderKeyName(item.Name) {
			continue
		}
		result = append(result, item)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

func runtimeEnvMap(items []EnvVar) map[string]string {
	env := make(map[string]string, len(items))
	for _, item := range normalizeEnvItems(items) {
		name := strings.TrimSpace(item.Name)
		if name == "" || llmProviderKeyName(name) {
			continue
		}
		env[name] = item.Value
	}
	if len(env) == 0 {
		return nil
	}
	return env
}

func managedRuntimeEnvMap(items []EnvVar) map[string]string {
	env := make(map[string]string, len(items))
	for _, item := range normalizeEnvItems(items) {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		env[name] = item.Value
	}
	if len(env) == 0 {
		return nil
	}
	return env
}
