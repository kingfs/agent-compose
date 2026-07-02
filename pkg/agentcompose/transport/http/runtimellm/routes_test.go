package runtimellm

import (
	"net/http"
	"net/url"
	"testing"
)

func TestIsFacadeRequestMatchesSupportedRuntimeLLMRoutes(t *testing.T) {
	for _, tc := range []struct {
		name   string
		method string
		path   string
		want   bool
	}{
		{name: "responses", method: http.MethodPost, path: "/api/runtime/sessions/session-1/llm/openai/v1/responses", want: true},
		{name: "chat completions", method: http.MethodPost, path: "/api/runtime/sessions/session-1/llm/openai/v1/chat/completions", want: true},
		{name: "anthropic messages", method: http.MethodPost, path: "/api/runtime/sessions/session-1/llm/anthropic/v1/messages", want: true},
		{name: "wrong method", method: http.MethodGet, path: "/api/runtime/sessions/session-1/llm/openai/v1/responses"},
		{name: "missing session", method: http.MethodPost, path: "/api/runtime/sessions//llm/openai/v1/responses"},
		{name: "extra suffix", method: http.MethodPost, path: "/api/runtime/sessions/session-1/llm/openai/v1/responses/extra"},
		{name: "unmanaged route", method: http.MethodPost, path: "/api/runtime/sessions/session-1/llm/openai/v1/models"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			req := &http.Request{Method: tc.method, URL: &url.URL{Path: tc.path}}
			if got := IsFacadeRequest(req); got != tc.want {
				t.Fatalf("IsFacadeRequest(%s %s) = %v, want %v", tc.method, tc.path, got, tc.want)
			}
		})
	}
}
