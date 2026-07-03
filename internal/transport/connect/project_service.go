package connecttransport

import (
	"context"
	"net/http"

	projectdomain "agent-compose/internal/project"
	agentcomposev2 "agent-compose/proto/agentcompose/v2"
	"agent-compose/proto/agentcompose/v2/agentcomposev2connect"
	"connectrpc.com/connect"
)

type ProjectService interface {
	ValidateProject(context.Context, *connect.Request[agentcomposev2.ValidateProjectRequest]) (*connect.Response[agentcomposev2.ValidateProjectResponse], error)
	ApplyProject(context.Context, *connect.Request[agentcomposev2.ApplyProjectRequest]) (*connect.Response[agentcomposev2.ApplyProjectResponse], error)
	GetProject(context.Context, projectdomain.GetProjectRequest) (projectdomain.GetProjectResult, error)
	ListProjects(context.Context, projectdomain.ListProjectsRequest) (projectdomain.ListProjectsResult, error)
	RemoveProject(context.Context, projectdomain.RemoveProjectRequest) (projectdomain.RemoveProjectResult, error)
	WatchProject(context.Context, *connect.Request[agentcomposev2.WatchProjectRequest], *connect.ServerStream[agentcomposev2.WatchProjectResponse]) error
}

type projectServiceHandler struct {
	agentcomposev2connect.UnimplementedProjectServiceHandler

	service ProjectService
}

func NewProjectServiceHandler(service ProjectService, opts ...connect.HandlerOption) (string, http.Handler) {
	return agentcomposev2connect.NewProjectServiceHandler(projectServiceHandler{service: service}, opts...)
}

func (h projectServiceHandler) ValidateProject(ctx context.Context, req *connect.Request[agentcomposev2.ValidateProjectRequest]) (*connect.Response[agentcomposev2.ValidateProjectResponse], error) {
	return h.service.ValidateProject(ctx, req)
}

func (h projectServiceHandler) ApplyProject(ctx context.Context, req *connect.Request[agentcomposev2.ApplyProjectRequest]) (*connect.Response[agentcomposev2.ApplyProjectResponse], error) {
	return h.service.ApplyProject(ctx, req)
}

func (h projectServiceHandler) GetProject(ctx context.Context, req *connect.Request[agentcomposev2.GetProjectRequest]) (*connect.Response[agentcomposev2.GetProjectResponse], error) {
	result, err := h.service.GetProject(ctx, GetProjectRequestFromProto(req.Msg))
	if err != nil {
		return nil, connect.NewError(ProjectQueryConnectCode(err), err)
	}
	resp, err := GetProjectResponseFromResult(result)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(resp), nil
}

func (h projectServiceHandler) ListProjects(ctx context.Context, req *connect.Request[agentcomposev2.ListProjectsRequest]) (*connect.Response[agentcomposev2.ListProjectsResponse], error) {
	result, err := h.service.ListProjects(ctx, ListProjectsRequestFromProto(req.Msg))
	if err != nil {
		return nil, connect.NewError(ProjectQueryConnectCode(err), err)
	}
	return connect.NewResponse(ListProjectsResponseFromResult(result)), nil
}

func (h projectServiceHandler) RemoveProject(ctx context.Context, req *connect.Request[agentcomposev2.RemoveProjectRequest]) (*connect.Response[agentcomposev2.RemoveProjectResponse], error) {
	result, err := h.service.RemoveProject(ctx, RemoveProjectRequestFromProto(req.Msg))
	if err != nil {
		return nil, connect.NewError(ProjectQueryConnectCode(err), err)
	}
	return connect.NewResponse(RemoveProjectResponseFromResult(result)), nil
}

func (h projectServiceHandler) WatchProject(ctx context.Context, req *connect.Request[agentcomposev2.WatchProjectRequest], stream *connect.ServerStream[agentcomposev2.WatchProjectResponse]) error {
	return h.service.WatchProject(ctx, req, stream)
}
