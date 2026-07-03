package app

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/samber/do/v2"

	projectdomain "agent-compose/internal/project"
	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

type ProjectService struct {
	service *Service
}

func NewProjectService(di do.Injector) (*ProjectService, error) {
	return NewProjectServiceFromService(do.MustInvoke[*Service](di)), nil
}

func NewProjectServiceFromService(service *Service) *ProjectService {
	return &ProjectService{service: service}
}

func (p *ProjectService) ValidateProject(ctx context.Context, req *connect.Request[agentcomposev2.ValidateProjectRequest]) (*connect.Response[agentcomposev2.ValidateProjectResponse], error) {
	normalized, issues, err := normalizeProjectServiceSpec(req.Msg.GetSpec(), req.Msg.GetSource(), req.Msg.GetExpectedSpecHash())
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	if len(issues) > 0 {
		return connect.NewResponse(&agentcomposev2.ValidateProjectResponse{
			Valid:    false,
			Issues:   issues,
			SpecHash: specHashOrEmpty(normalized),
		}), nil
	}
	if issues := p.validateProjectManagedAgentDefinitions(normalized); len(issues) > 0 {
		return connect.NewResponse(&agentcomposev2.ValidateProjectResponse{
			Valid:    false,
			Issues:   issues,
			SpecHash: normalized.specHash,
		}), nil
	}
	if issues := p.validateProjectManagedSchedulers(ctx, normalized); len(issues) > 0 {
		return connect.NewResponse(&agentcomposev2.ValidateProjectResponse{
			Valid:    false,
			Issues:   issues,
			SpecHash: normalized.specHash,
		}), nil
	}
	return connect.NewResponse(&agentcomposev2.ValidateProjectResponse{
		Valid:    true,
		SpecHash: normalized.specHash,
	}), nil
}

func (p *ProjectService) ApplyProject(ctx context.Context, req *connect.Request[agentcomposev2.ApplyProjectRequest]) (*connect.Response[agentcomposev2.ApplyProjectResponse], error) {
	result, err := p.applyProjectWorkflow(ctx, req.Msg)
	if err != nil {
		return nil, connect.NewError(projectApplyConnectCode(err), err)
	}
	return connect.NewResponse(projectApplyResponseFromResult(result)), nil
}

func (p *ProjectService) GetProject(ctx context.Context, req projectdomain.GetProjectRequest) (projectdomain.GetProjectResult, error) {
	return p.projectQueryUsecase().GetProject(ctx, req)
}

func (p *ProjectService) ListProjects(ctx context.Context, req projectdomain.ListProjectsRequest) (projectdomain.ListProjectsResult, error) {
	return p.projectQueryUsecase().ListProjects(ctx, req)
}

func (p *ProjectService) RemoveProject(ctx context.Context, req projectdomain.RemoveProjectRequest) (projectdomain.RemoveProjectResult, error) {
	return p.projectQueryUsecase().RemoveProject(ctx, req)
}

func (p *ProjectService) projectQueryUsecase() *projectdomain.QueryUsecase {
	return projectdomain.NewQueryUsecase(projectdomain.QueryUsecaseOptions{
		Store:           p.service.configDB,
		Runtime:         projectRuntimeAdapter{service: p.service},
		StableProjectID: StableProjectID,
	})
}

type projectRuntimeAdapter struct {
	service *Service
}

func (a projectRuntimeAdapter) DownProject(ctx context.Context, project ProjectRecord) ([]projectdomain.Change, error) {
	changes, err := a.service.downProject(ctx, project)
	if err != nil {
		return nil, err
	}
	return projectQueryChangesFromProto(changes), nil
}

func projectQueryChangesFromProto(changes []*agentcomposev2.ProjectChange) []projectdomain.Change {
	if len(changes) == 0 {
		return nil
	}
	result := make([]projectdomain.Change, 0, len(changes))
	for _, change := range changes {
		if change == nil {
			continue
		}
		result = append(result, projectdomain.Change{
			Kind:     projectChangeKindFromProto(change.GetAction()),
			Resource: change.GetResourceType(),
			ID:       change.GetResourceId(),
			Name:     change.GetName(),
			Message:  change.GetMessage(),
		})
	}
	return result
}

func (s *ProjectService) WatchProject(ctx context.Context, req *connect.Request[agentcomposev2.WatchProjectRequest], stream *connect.ServerStream[agentcomposev2.WatchProjectResponse]) error {
	_, _, _ = ctx, req, stream
	return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("agentcompose.v2.ProjectService.WatchProject is not implemented"))
}
