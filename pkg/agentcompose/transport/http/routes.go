package httptransport

import (
	"context"
	"net"
	"net/http"
	"net/http/httputil"
	"net/url"
	"strings"
	"time"

	sessionmodel "agent-compose/pkg/agentcompose/session"
	workspacehttp "agent-compose/pkg/agentcompose/transport/http/workspace"
	webhookhandler "agent-compose/pkg/agentcompose/webhook"
	driverpkg "agent-compose/pkg/driver"

	"github.com/labstack/echo/v4"
)

type Services interface {
	ProxyService
	RuntimeLLMService
	WebhookService
	workspacehttp.FileWorkspaceLoader
}

type ProxyService interface {
	JupyterProxyBasePath() string
	EnsureSessionProxyReady(ctx context.Context, sessionID string) (sessionmodel.ProxyState, error)
	GetProxyState(sessionID string) (sessionmodel.ProxyState, error)
}

type RuntimeLLMService interface {
	HandleRuntimeLLMResponses(c echo.Context) error
	HandleRuntimeLLMChatCompletions(c echo.Context) error
	HandleRuntimeLLMAnthropicMessages(c echo.Context) error
}

type WebhookService interface {
	WebhookBodyLimitBytes() int64
	WebhookStore() webhookhandler.Store
}

func RegisterRoutes(app *echo.Echo, service Services) {
	RegisterWebhookRoutes(app, service)
	RegisterRuntimeLLMFacadeRoutes(app, service)
	RegisterProxyRoutes(app, service)
	RegisterWorkspaceRoutes(app, service)
}

func RegisterWebhookRoutes(app *echo.Echo, service WebhookService) {
	webhookhandler.RegisterRoutes(app, webhookhandler.NewHandler(service.WebhookBodyLimitBytes(), service.WebhookStore()))
}

func RegisterRuntimeLLMFacadeRoutes(app *echo.Echo, service RuntimeLLMService) {
	app.POST("/api/runtime/sessions/:session_id/llm/openai/v1/responses", service.HandleRuntimeLLMResponses)
	app.POST("/api/runtime/sessions/:session_id/llm/openai/v1/chat/completions", service.HandleRuntimeLLMChatCompletions)
	app.POST("/api/runtime/sessions/:session_id/llm/anthropic/v1/messages", service.HandleRuntimeLLMAnthropicMessages)
}

func RegisterProxyRoutes(app *echo.Echo, service ProxyService) {
	base := strings.TrimRight(service.JupyterProxyBasePath(), "/")
	app.GET(base+"/:sessionID", func(c echo.Context) error {
		proxyState, err := service.EnsureSessionProxyReady(c.Request().Context(), c.Param("sessionID"))
		if err != nil {
			return echo.NewHTTPError(http.StatusBadGateway, err.Error())
		}
		location := strings.TrimRight(proxyState.ProxyPath, "/")
		if proxyState.Token != "" {
			location += "?token=" + url.QueryEscape(proxyState.Token)
		}
		return c.Redirect(http.StatusTemporaryRedirect, location)
	})
	app.Any(base+"/:sessionID/*", func(c echo.Context) error {
		sessionID := c.Param("sessionID")
		if !JupyterTargetReachable(func() sessionmodel.ProxyState {
			proxyState, err := service.GetProxyState(sessionID)
			if err != nil {
				return sessionmodel.ProxyState{}
			}
			return proxyState
		}(), 250*time.Millisecond) {
			if _, err := service.EnsureSessionProxyReady(c.Request().Context(), sessionID); err != nil {
				return echo.NewHTTPError(http.StatusBadGateway, err.Error())
			}
		}
		proxyState, err := service.GetProxyState(sessionID)
		if err != nil {
			return echo.NewHTTPError(http.StatusNotFound, err.Error())
		}
		target, err := url.Parse("http://" + driverpkg.JupyterConnectAddress(toDriverProxyState(proxyState)))
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError, err.Error())
		}
		proxy := &httputil.ReverseProxy{
			Rewrite: func(req *httputil.ProxyRequest) {
				req.SetURL(target)
				req.SetXForwarded()
				req.Out.Host = target.Host
				req.Out.URL.Path = req.In.URL.Path
				req.Out.URL.RawPath = req.Out.URL.Path
				req.Out.URL.RawQuery = req.In.URL.RawQuery
			},
			ErrorHandler: func(rw http.ResponseWriter, req *http.Request, proxyErr error) {
				rw.WriteHeader(http.StatusBadGateway)
				_, _ = rw.Write([]byte(proxyErr.Error()))
			},
		}
		proxy.ServeHTTP(c.Response(), c.Request())
		return nil
	})
}

func RegisterWorkspaceRoutes(app *echo.Echo, service workspacehttp.FileWorkspaceLoader) {
	workspacehttp.RegisterRoutes(app, service)
}

func JupyterTargetReachable(proxyState sessionmodel.ProxyState, timeout time.Duration) bool {
	_, port := driverpkg.JupyterConnectTarget(toDriverProxyState(proxyState))
	if port <= 0 {
		return false
	}
	conn, err := net.DialTimeout("tcp", driverpkg.JupyterConnectAddress(toDriverProxyState(proxyState)), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

func toDriverProxyState(state sessionmodel.ProxyState) driverpkg.ProxyState {
	return driverpkg.ProxyState{
		ProxyPath:  state.ProxyPath,
		GuestHost:  state.GuestHost,
		HostPort:   state.HostPort,
		GuestPort:  state.GuestPort,
		JupyterURL: state.JupyterURL,
		Token:      state.Token,
	}
}
