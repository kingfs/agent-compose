package llm

import (
	appconfig "agent-compose/pkg/config"
	"context"
	"reflect"
	"testing"
)

func TestLLMNormalizationHelpers(t *testing.T) {
	if got := normalizeLLMWireAPI(" chat-completion "); got != llmAPIProtocolChatCompletions {
		t.Fatalf("wire api = %q", got)
	}
	if got := normalizeLLMWireAPI(""); got != llmAPIProtocolResponses {
		t.Fatalf("default wire api = %q", got)
	}
	if got := normalizeLLMProviderType(" claude "); got != llmProviderFamilyAnthropic {
		t.Fatalf("anthropic provider type = %q", got)
	}
	if got := normalizeLLMProviderType(" openai-compatible "); got != llmProviderFamilyOpenAI {
		t.Fatalf("openai provider type = %q", got)
	}
	if got := normalizeLLMAPIBaseURL("https://example.test/v1/responses/", llmAPIProtocolResponses); got != "https://example.test/v1" {
		t.Fatalf("openai base url = %q", got)
	}
	if got := normalizeAnthropicAPIBaseURL("https://api.anthropic.com/v1/messages"); got != "https://api.anthropic.com/v1" {
		t.Fatalf("anthropic base url = %q", got)
	}
}

func TestDefaultLLMEnvProviderLookupUsesSourceMajorPriority(t *testing.T) {
	t.Setenv("LLM_API_KEY", "process-key")
	t.Setenv("OPENAI_API_KEY", "process-openai-key")

	store := NewStore(nil, func(context.Context) ([]GlobalEnvVar, error) {
		return []GlobalEnvVar{{Name: "OPENAI_API_KEY", Value: "global-openai-key"}}, nil
	})
	lookup := defaultLLMEnvProviderLookup(context.Background(), &appconfig.Config{LLMAPIKey: "config-key"}, store)

	if got := lookup("LLM_API_KEY", "OPENAI_API_KEY"); got != "global-openai-key" {
		t.Fatalf("lookup = %q", got)
	}
}

func TestSessionEnvProviderHelpersNormalizeCaseAndFilterManagedKeys(t *testing.T) {
	items := []EnvVar{
		{Name: " llm_api_key ", Value: " openai-key "},
		{Name: "ANTHROPIC_AUTH_TOKEN", Value: " anthropic-token "},
		{Name: "KEEP_ME", Value: "kept", Secret: true},
		{Name: " ", Value: "ignored"},
	}

	if !hasSessionEnvProviderInput(items) {
		t.Fatal("expected session env provider input")
	}
	if !envHasProviderKeyForFamily(items, llmProviderFamilyOpenAI) {
		t.Fatal("expected openai provider key")
	}
	if !envHasProviderKeyForFamily(items, llmProviderFamilyAnthropic) {
		t.Fatal("expected anthropic provider key")
	}
	if got := lookupEnvItemValue(items, "LLM_API_KEY"); got != "openai-key" {
		t.Fatalf("lookupEnvItemValue = %q", got)
	}
	if got := filterPersistedRuntimeEnv(items); !reflect.DeepEqual(got, []EnvVar{{Name: "KEEP_ME", Value: "kept", Secret: true}}) {
		t.Fatalf("filterPersistedRuntimeEnv = %#v", got)
	}
	if got := runtimeEnvMap(items); !reflect.DeepEqual(got, map[string]string{"KEEP_ME": "kept"}) {
		t.Fatalf("runtimeEnvMap = %#v", got)
	}
}

func TestProviderHeaderValidationFiltersManagedHeaders(t *testing.T) {
	headers, err := providerForwardHeaders(LLMProvider{
		APIKey:      "secret",
		AuthHeader:  "Authorization",
		AuthScheme:  "Bearer",
		HeadersJSON: `{"authorization":"custom","content-type":"application/json","x-extra":"ok"}`,
	})
	if err != nil {
		t.Fatalf("providerForwardHeaders returned error: %v", err)
	}
	if got := headers.Get("Authorization"); got != "Bearer secret" {
		t.Fatalf("authorization = %q", got)
	}
	if got := headers.Get("Content-Type"); got != "" {
		t.Fatalf("content-type = %q", got)
	}
	if got := headers.Get("X-Extra"); got != "ok" {
		t.Fatalf("x-extra = %q", got)
	}
}
