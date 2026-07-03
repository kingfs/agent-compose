package app

import (
	"log/slog"

	"github.com/labstack/echo/v4"
	"github.com/samber/do/v2"
)

func Setup(di do.Injector) {
	Register(di)
	if err := StartBackground(di); err != nil {
		slog.Error("failed to start agent-compose background managers", "error", err)
	}
}

func Register(di do.Injector) {
	registerProviders(di)

	app := do.MustInvoke[*echo.Echo](di)
	service := do.MustInvoke[*Service](di)

	registerConnectRoutes(app, service)
	registerHTTPRoutes(app, service)
}
