package app

import (
	"github.com/labstack/echo/v4"

	httptransport "agent-compose/internal/transport/http"
)

func registerProxyRoutes(app *echo.Echo, service *Service) {
	httptransport.RegisterProxyRoutes(app, service)
}
