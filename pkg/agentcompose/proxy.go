package agentcompose

import (
	"context"
	"time"

	sessionmodel "agent-compose/pkg/agentcompose/session"
	transporthttp "agent-compose/pkg/agentcompose/transport/http"
	"github.com/labstack/echo/v4"
)

func registerProxyRoutes(app *echo.Echo, service *Service) {
	transporthttp.RegisterProxyRoutes(app, service)
}

func (s *Service) JupyterProxyBasePath() string {
	return s.config.JupyterProxyBasePath
}

func (s *Service) EnsureSessionProxyReady(ctx context.Context, sessionID string) (sessionmodel.ProxyState, error) {
	_, proxyState, err := s.ensureSessionProxyReady(ctx, sessionID)
	return proxyState, err
}

func (s *Service) GetProxyState(sessionID string) (sessionmodel.ProxyState, error) {
	return s.store.GetProxyState(sessionID)
}

func jupyterTargetReachable(proxyState ProxyState, timeout time.Duration) bool {
	return transporthttp.JupyterTargetReachable(proxyState, timeout)
}
