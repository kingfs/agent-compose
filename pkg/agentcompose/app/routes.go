package app

import (
	"github.com/labstack/echo/v4"
	"github.com/samber/do/v2"

	connectv1 "agent-compose/pkg/agentcompose/transport/connectv1"
	connectv2 "agent-compose/pkg/agentcompose/transport/connectv2"
	transporthttp "agent-compose/pkg/agentcompose/transport/http"
)

type RouteService interface {
	connectv1.Services
	connectv2.Services
	transporthttp.Services
}

func RegisterRoutes(di do.Injector, service RouteService) {
	app := do.MustInvoke[*echo.Echo](di)
	connectv1.RegisterRoutes(app, connectv1.NewHandler(service))
	connectv2.RegisterRoutes(app, connectv2.NewHandler(service))
	transporthttp.RegisterRoutes(app, service)
}
