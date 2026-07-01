package connectv1

import (
	"github.com/labstack/echo/v4"

	"agent-compose/proto/agentcompose/v1/agentcomposev1connect"
)

type Services interface {
	agentcomposev1connect.SessionServiceHandler
	agentcomposev1connect.KernelServiceHandler
	agentcomposev1connect.AgentServiceHandler
	agentcomposev1connect.AgentDefinitionServiceHandler
	agentcomposev1connect.LLMServiceHandler
	agentcomposev1connect.ConfigServiceHandler
	agentcomposev1connect.LoaderServiceHandler
	agentcomposev1connect.DashboardServiceHandler
	agentcomposev1connect.CapabilityServiceHandler
}

type Handler struct {
	Services
}

func NewHandler(services Services) *Handler {
	return &Handler{Services: services}
}

func RegisterRoutes(app *echo.Echo, handler *Handler) {
	path, httpHandler := agentcomposev1connect.NewSessionServiceHandler(handler)
	app.Any(path+"*", echo.WrapHandler(httpHandler))
	path, httpHandler = agentcomposev1connect.NewKernelServiceHandler(handler)
	app.Any(path+"*", echo.WrapHandler(httpHandler))
	path, httpHandler = agentcomposev1connect.NewAgentServiceHandler(handler)
	app.Any(path+"*", echo.WrapHandler(httpHandler))
	path, httpHandler = agentcomposev1connect.NewAgentDefinitionServiceHandler(handler)
	app.Any(path+"*", echo.WrapHandler(httpHandler))
	path, httpHandler = agentcomposev1connect.NewLLMServiceHandler(handler)
	app.Any(path+"*", echo.WrapHandler(httpHandler))
	path, httpHandler = agentcomposev1connect.NewConfigServiceHandler(handler)
	app.Any(path+"*", echo.WrapHandler(httpHandler))
	path, httpHandler = agentcomposev1connect.NewLoaderServiceHandler(handler)
	app.Any(path+"*", echo.WrapHandler(httpHandler))
	path, httpHandler = agentcomposev1connect.NewDashboardServiceHandler(handler)
	app.Any(path+"*", echo.WrapHandler(httpHandler))
	path, httpHandler = agentcomposev1connect.NewCapabilityServiceHandler(handler)
	app.Any(path+"*", echo.WrapHandler(httpHandler))
}
