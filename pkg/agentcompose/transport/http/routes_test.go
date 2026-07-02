package httptransport

import (
	"context"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	sessionmodel "agent-compose/pkg/agentcompose/session"

	"github.com/labstack/echo/v4"
)

type proxyRouteService struct {
	state      sessionmodel.ProxyState
	ensureHits int
}

func (s *proxyRouteService) JupyterProxyBasePath() string {
	return "/agent-compose/session"
}

func (s *proxyRouteService) EnsureSessionProxyReady(context.Context, string) (sessionmodel.ProxyState, error) {
	s.ensureHits++
	return s.state, nil
}

func (s *proxyRouteService) GetProxyState(string) (sessionmodel.ProxyState, error) {
	return s.state, nil
}

func TestRegisterRuntimeLLMFacadeRoutes(t *testing.T) {
	app := echo.New()
	service := &runtimeLLMRouteService{}
	RegisterRuntimeLLMFacadeRoutes(app, service)

	for _, tc := range []struct {
		path string
		want string
	}{
		{path: "/api/runtime/sessions/session-1/llm/openai/v1/responses", want: "responses"},
		{path: "/api/runtime/sessions/session-1/llm/openai/v1/chat/completions", want: "chat"},
		{path: "/api/runtime/sessions/session-1/llm/anthropic/v1/messages", want: "anthropic"},
	} {
		req := httptest.NewRequest(http.MethodPost, tc.path, nil)
		rec := httptest.NewRecorder()
		app.ServeHTTP(rec, req)
		if rec.Code != http.StatusNoContent {
			t.Fatalf("%s status = %d, want 204", tc.path, rec.Code)
		}
		if service.last != tc.want {
			t.Fatalf("%s handler = %q, want %q", tc.path, service.last, tc.want)
		}
	}
}

func TestRegisterProxyRoutesRedirectsToReadySessionProxyPath(t *testing.T) {
	app := echo.New()
	service := &proxyRouteService{state: sessionmodel.ProxyState{ProxyPath: "/agent-compose/session/session-1/lab", Token: "token with space"}}
	RegisterProxyRoutes(app, service)

	req := httptest.NewRequest(http.MethodGet, "/agent-compose/session/session-1", nil)
	rec := httptest.NewRecorder()
	app.ServeHTTP(rec, req)

	if rec.Code != http.StatusTemporaryRedirect {
		t.Fatalf("status = %d, want 307", rec.Code)
	}
	if location := rec.Header().Get(echo.HeaderLocation); location != "/agent-compose/session/session-1/lab?token=token+with+space" {
		t.Fatalf("location = %q", location)
	}
	if service.ensureHits != 1 {
		t.Fatalf("EnsureSessionProxyReady calls = %d, want 1", service.ensureHits)
	}
}

func TestJupyterTargetReachableUsesProxyStateAddress(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer func() { _ = listener.Close() }()
	port := listener.Addr().(*net.TCPAddr).Port

	if !JupyterTargetReachable(sessionmodel.ProxyState{GuestHost: "127.0.0.1", HostPort: port, GuestPort: 8888}, 250*time.Millisecond) {
		t.Fatalf("expected loopback host port target to be reachable")
	}
	if JupyterTargetReachable(sessionmodel.ProxyState{}, 0) {
		t.Fatalf("empty proxy state was reachable")
	}
}

type runtimeLLMRouteService struct {
	last string
}

func (s *runtimeLLMRouteService) HandleRuntimeLLMResponses(c echo.Context) error {
	s.last = "responses"
	return c.NoContent(http.StatusNoContent)
}

func (s *runtimeLLMRouteService) HandleRuntimeLLMChatCompletions(c echo.Context) error {
	s.last = "chat"
	return c.NoContent(http.StatusNoContent)
}

func (s *runtimeLLMRouteService) HandleRuntimeLLMAnthropicMessages(c echo.Context) error {
	s.last = "anthropic"
	return c.NoContent(http.StatusNoContent)
}
