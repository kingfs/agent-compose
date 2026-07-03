package app

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/samber/do/v2"

	agentcomposev2 "agent-compose/proto/agentcompose/v2"
	"agent-compose/proto/agentcompose/v2/agentcomposev2connect"
)

var _ agentcomposev2connect.ProjectServiceHandler = (*ProjectService)(nil)

type ProjectService struct {
	service *Service
}

func NewProjectService(di do.Injector) (*ProjectService, error) {
	return NewProjectServiceFromService(do.MustInvoke[*Service](di)), nil
}

func NewProjectServiceFromService(service *Service) *ProjectService {
	return &ProjectService{service: service}
}

func (s *ProjectService) ValidateProject(ctx context.Context, req *connect.Request[agentcomposev2.ValidateProjectRequest]) (*connect.Response[agentcomposev2.ValidateProjectResponse], error) {
	return s.service.ValidateProject(ctx, req)
}

func (s *ProjectService) ApplyProject(ctx context.Context, req *connect.Request[agentcomposev2.ApplyProjectRequest]) (*connect.Response[agentcomposev2.ApplyProjectResponse], error) {
	return s.service.ApplyProject(ctx, req)
}

func (s *ProjectService) GetProject(ctx context.Context, req *connect.Request[agentcomposev2.GetProjectRequest]) (*connect.Response[agentcomposev2.GetProjectResponse], error) {
	return s.service.GetProject(ctx, req)
}

func (s *ProjectService) ListProjects(ctx context.Context, req *connect.Request[agentcomposev2.ListProjectsRequest]) (*connect.Response[agentcomposev2.ListProjectsResponse], error) {
	return s.service.ListProjects(ctx, req)
}

func (s *ProjectService) RemoveProject(ctx context.Context, req *connect.Request[agentcomposev2.RemoveProjectRequest]) (*connect.Response[agentcomposev2.RemoveProjectResponse], error) {
	return s.service.RemoveProject(ctx, req)
}

func (s *ProjectService) WatchProject(ctx context.Context, req *connect.Request[agentcomposev2.WatchProjectRequest], stream *connect.ServerStream[agentcomposev2.WatchProjectResponse]) error {
	_, _, _ = ctx, req, stream
	return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("agentcompose.v2.ProjectService.WatchProject is not implemented"))
}
