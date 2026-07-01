package connectv2

import (
	"context"

	"connectrpc.com/connect"
	"github.com/labstack/echo/v4"

	agentcomposev2 "agent-compose/proto/agentcompose/v2"
	"agent-compose/proto/agentcompose/v2/agentcomposev2connect"
)

type Services interface {
	ProjectService
	RunService
	ExecService
	ImageService
}

type ProjectService interface {
	ValidateProject(context.Context, *connect.Request[agentcomposev2.ValidateProjectRequest]) (*connect.Response[agentcomposev2.ValidateProjectResponse], error)
	ApplyProject(context.Context, *connect.Request[agentcomposev2.ApplyProjectRequest]) (*connect.Response[agentcomposev2.ApplyProjectResponse], error)
	GetProject(context.Context, *connect.Request[agentcomposev2.GetProjectRequest]) (*connect.Response[agentcomposev2.GetProjectResponse], error)
	ListProjects(context.Context, *connect.Request[agentcomposev2.ListProjectsRequest]) (*connect.Response[agentcomposev2.ListProjectsResponse], error)
	RemoveProject(context.Context, *connect.Request[agentcomposev2.RemoveProjectRequest]) (*connect.Response[agentcomposev2.RemoveProjectResponse], error)
}

type RunService interface {
	RunAgent(context.Context, *connect.Request[agentcomposev2.RunAgentRequest]) (*connect.Response[agentcomposev2.RunAgentResponse], error)
	RunAgentStream(context.Context, *connect.Request[agentcomposev2.RunAgentRequest], *connect.ServerStream[agentcomposev2.RunAgentStreamResponse]) error
	GetRun(context.Context, *connect.Request[agentcomposev2.GetRunRequest]) (*connect.Response[agentcomposev2.GetRunResponse], error)
	ListRuns(context.Context, *connect.Request[agentcomposev2.ListRunsRequest]) (*connect.Response[agentcomposev2.ListRunsResponse], error)
	StopRun(context.Context, *connect.Request[agentcomposev2.StopRunRequest]) (*connect.Response[agentcomposev2.StopRunResponse], error)
}

type ExecService interface {
	Exec(context.Context, *connect.Request[agentcomposev2.ExecRequest]) (*connect.Response[agentcomposev2.ExecResponse], error)
	ExecStream(context.Context, *connect.Request[agentcomposev2.ExecRequest], *connect.ServerStream[agentcomposev2.ExecStreamResponse]) error
}

type ImageService interface {
	ListImages(context.Context, *connect.Request[agentcomposev2.ListImagesRequest]) (*connect.Response[agentcomposev2.ListImagesResponse], error)
	PullImage(context.Context, *connect.Request[agentcomposev2.PullImageRequest]) (*connect.Response[agentcomposev2.PullImageResponse], error)
	InspectImage(context.Context, *connect.Request[agentcomposev2.InspectImageRequest]) (*connect.Response[agentcomposev2.InspectImageResponse], error)
	RemoveImage(context.Context, *connect.Request[agentcomposev2.RemoveImageRequest]) (*connect.Response[agentcomposev2.RemoveImageResponse], error)
}

type Handler struct {
	agentcomposev2connect.UnimplementedProjectServiceHandler
	services Services
}

func NewHandler(services Services) *Handler {
	return &Handler{services: services}
}

func RegisterRoutes(app *echo.Echo, handler *Handler) {
	path, httpHandler := agentcomposev2connect.NewProjectServiceHandler(handler)
	app.Any(path+"*", echo.WrapHandler(httpHandler))
	path, httpHandler = agentcomposev2connect.NewRunServiceHandler(handler)
	app.Any(path+"*", echo.WrapHandler(httpHandler))
	path, httpHandler = agentcomposev2connect.NewExecServiceHandler(handler)
	app.Any(path+"*", echo.WrapHandler(httpHandler))
	path, httpHandler = agentcomposev2connect.NewImageServiceHandler(handler)
	app.Any(path+"*", echo.WrapHandler(httpHandler))
}

func (h *Handler) ValidateProject(ctx context.Context, req *connect.Request[agentcomposev2.ValidateProjectRequest]) (*connect.Response[agentcomposev2.ValidateProjectResponse], error) {
	return h.services.ValidateProject(ctx, req)
}

func (h *Handler) ApplyProject(ctx context.Context, req *connect.Request[agentcomposev2.ApplyProjectRequest]) (*connect.Response[agentcomposev2.ApplyProjectResponse], error) {
	return h.services.ApplyProject(ctx, req)
}

func (h *Handler) GetProject(ctx context.Context, req *connect.Request[agentcomposev2.GetProjectRequest]) (*connect.Response[agentcomposev2.GetProjectResponse], error) {
	return h.services.GetProject(ctx, req)
}

func (h *Handler) ListProjects(ctx context.Context, req *connect.Request[agentcomposev2.ListProjectsRequest]) (*connect.Response[agentcomposev2.ListProjectsResponse], error) {
	return h.services.ListProjects(ctx, req)
}

func (h *Handler) RemoveProject(ctx context.Context, req *connect.Request[agentcomposev2.RemoveProjectRequest]) (*connect.Response[agentcomposev2.RemoveProjectResponse], error) {
	return h.services.RemoveProject(ctx, req)
}

func (h *Handler) RunAgent(ctx context.Context, req *connect.Request[agentcomposev2.RunAgentRequest]) (*connect.Response[agentcomposev2.RunAgentResponse], error) {
	return h.services.RunAgent(ctx, req)
}

func (h *Handler) RunAgentStream(ctx context.Context, req *connect.Request[agentcomposev2.RunAgentRequest], stream *connect.ServerStream[agentcomposev2.RunAgentStreamResponse]) error {
	return h.services.RunAgentStream(ctx, req, stream)
}

func (h *Handler) GetRun(ctx context.Context, req *connect.Request[agentcomposev2.GetRunRequest]) (*connect.Response[agentcomposev2.GetRunResponse], error) {
	return h.services.GetRun(ctx, req)
}

func (h *Handler) ListRuns(ctx context.Context, req *connect.Request[agentcomposev2.ListRunsRequest]) (*connect.Response[agentcomposev2.ListRunsResponse], error) {
	return h.services.ListRuns(ctx, req)
}

func (h *Handler) StopRun(ctx context.Context, req *connect.Request[agentcomposev2.StopRunRequest]) (*connect.Response[agentcomposev2.StopRunResponse], error) {
	return h.services.StopRun(ctx, req)
}

func (h *Handler) Exec(ctx context.Context, req *connect.Request[agentcomposev2.ExecRequest]) (*connect.Response[agentcomposev2.ExecResponse], error) {
	return h.services.Exec(ctx, req)
}

func (h *Handler) ExecStream(ctx context.Context, req *connect.Request[agentcomposev2.ExecRequest], stream *connect.ServerStream[agentcomposev2.ExecStreamResponse]) error {
	return h.services.ExecStream(ctx, req, stream)
}

func (h *Handler) ListImages(ctx context.Context, req *connect.Request[agentcomposev2.ListImagesRequest]) (*connect.Response[agentcomposev2.ListImagesResponse], error) {
	return h.services.ListImages(ctx, req)
}

func (h *Handler) PullImage(ctx context.Context, req *connect.Request[agentcomposev2.PullImageRequest]) (*connect.Response[agentcomposev2.PullImageResponse], error) {
	return h.services.PullImage(ctx, req)
}

func (h *Handler) InspectImage(ctx context.Context, req *connect.Request[agentcomposev2.InspectImageRequest]) (*connect.Response[agentcomposev2.InspectImageResponse], error) {
	return h.services.InspectImage(ctx, req)
}

func (h *Handler) RemoveImage(ctx context.Context, req *connect.Request[agentcomposev2.RemoveImageRequest]) (*connect.Response[agentcomposev2.RemoveImageResponse], error) {
	return h.services.RemoveImage(ctx, req)
}
