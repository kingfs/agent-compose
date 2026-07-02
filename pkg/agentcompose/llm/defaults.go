package llm

import (
	"net/url"
	pathpkg "path"
	"strings"
)

func normalizeLLMWireAPI(value string) string {
	switch strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "_") {
	case "", llmAPIProtocolResponses:
		return llmAPIProtocolResponses
	case "chat", "chat_completion", llmAPIProtocolChatCompletions:
		return llmAPIProtocolChatCompletions
	case "message", llmAPIProtocolMessages:
		return llmAPIProtocolMessages
	default:
		return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "_")
	}
}

func normalizeLLMProviderType(value string) string {
	switch strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "_") {
	case "", "openai", "openai_compatible":
		return llmProviderFamilyOpenAI
	case "anthropic", "claude", "anthropic_messages":
		return llmProviderFamilyAnthropic
	default:
		return strings.ReplaceAll(strings.ToLower(strings.TrimSpace(value)), "-", "_")
	}
}

func normalizeOptionalLLMProviderType(value string) string {
	if strings.TrimSpace(value) == "" {
		return ""
	}
	return normalizeLLMProviderType(value)
}

func normalizeLLMAPIBaseURL(raw, wireAPI string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	cleanPath := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(cleanPath, "/responses"):
		parsed.Path = strings.TrimSuffix(cleanPath, "/responses")
	case strings.HasSuffix(cleanPath, "/chat/completions"):
		parsed.Path = strings.TrimSuffix(cleanPath, "/chat/completions")
	default:
		parsed.Path = cleanPath
	}
	return strings.TrimRight(parsed.String(), "/")
}

func llmEndpointForProvider(provider LLMProvider, wireAPI string) string {
	if normalizeLLMProviderType(provider.ProviderType) == llmProviderFamilyAnthropic {
		baseURL := normalizeAnthropicAPIBaseURL(provider.BaseURL)
		parsed, err := url.Parse(baseURL)
		if err != nil {
			return strings.TrimRight(baseURL, "/") + "/messages"
		}
		parsed.Path = pathpkg.Join(parsed.Path, "messages")
		return parsed.String()
	}
	baseURL := normalizeLLMAPIBaseURL(provider.BaseURL, wireAPI)
	if !llmProviderScopeIsConfigured(provider.Scope) {
		return normalizeLLMAPIEndpointForProtocol(baseURL, wireAPI)
	}
	return appendLLMAPIEndpointToBaseURL(baseURL, wireAPI)
}

func appendLLMAPIEndpointToBaseURL(baseURL, wireAPI string) string {
	baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	if baseURL == "" {
		return ""
	}
	parsed, err := url.Parse(baseURL)
	if err != nil {
		switch normalizeLLMWireAPI(wireAPI) {
		case llmAPIProtocolChatCompletions:
			return baseURL + "/v1/chat/completions"
		default:
			return baseURL + "/v1/responses"
		}
	}
	cleanPath := strings.TrimRight(parsed.Path, "/")
	switch normalizeLLMWireAPI(wireAPI) {
	case llmAPIProtocolChatCompletions:
		if cleanPath == "/v1" || strings.HasSuffix(cleanPath, "/v1") {
			joinLLMAPIBasePath(parsed, cleanPath, "chat/completions")
		} else {
			joinLLMAPIBasePath(parsed, cleanPath, "v1/chat/completions")
		}
	default:
		if cleanPath == "/v1" || strings.HasSuffix(cleanPath, "/v1") {
			joinLLMAPIBasePath(parsed, cleanPath, "responses")
		} else {
			joinLLMAPIBasePath(parsed, cleanPath, "v1/responses")
		}
	}
	return parsed.String()
}

func joinLLMAPIBasePath(parsed *url.URL, basePath, suffix string) {
	if parsed == nil {
		return
	}
	joined := pathpkg.Join(basePath, suffix)
	if parsed.Host != "" && !strings.HasPrefix(joined, "/") {
		joined = "/" + joined
	}
	parsed.Path = joined
}

func normalizeAnthropicAPIBaseURL(raw string) string {
	raw = strings.TrimRight(strings.TrimSpace(raw), "/")
	if raw == "" {
		return ""
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	cleanPath := strings.TrimRight(parsed.Path, "/")
	switch {
	case strings.HasSuffix(cleanPath, "/messages"):
		parsed.Path = strings.TrimSuffix(cleanPath, "/messages")
	case cleanPath == "":
		parsed.Path = "/v1"
	default:
		parsed.Path = cleanPath
	}
	return strings.TrimRight(parsed.String(), "/")
}
