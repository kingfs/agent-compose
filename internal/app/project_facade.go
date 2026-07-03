package app

import (
	"context"
	"fmt"

	"connectrpc.com/connect"
	"github.com/samber/do/v2"

	projectdomain "agent-compose/internal/project"
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

func (p *ProjectService) GetProject(ctx context.Context, req *connect.Request[agentcomposev2.GetProjectRequest]) (*connect.Response[agentcomposev2.GetProjectResponse], error) {
	result, err := p.projectQueryUsecase().GetProject(ctx, projectdomain.GetProjectRequest{
		Ref:         projectRefFromProto(req.Msg.GetProject()),
		IncludeSpec: req.Msg.GetIncludeSpec(),
	})
	if err != nil {
		return nil, connect.NewError(projectQueryConnectCode(err), err)
	}
	var spec *agentcomposev2.ProjectSpec
	if result.RevisionSpecJSON != "" {
		spec, err = decodeProjectRevisionSpec(result.RevisionSpecJSON)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("decode project %s revision %d: %w", result.Project.Name, result.Project.CurrentRevision, err))
		}
	}
	return connect.NewResponse(&agentcomposev2.GetProjectResponse{
		Project: projectResponse(result.Project, spec, result.Agents, result.Schedulers),
	}), nil
}

func (p *ProjectService) ListProjects(ctx context.Context, req *connect.Request[agentcomposev2.ListProjectsRequest]) (*connect.Response[agentcomposev2.ListProjectsResponse], error) {
	result, err := p.projectQueryUsecase().ListProjects(ctx, projectdomain.ListProjectsRequest{
		Query:          req.Msg.GetQuery(),
		IncludeRemoved: req.Msg.GetIncludeRemoved(),
		Offset:         int(req.Msg.GetOffset()),
		Limit:          int(req.Msg.GetLimit()),
	})
	if err != nil {
		return nil, connect.NewError(projectQueryConnectCode(err), err)
	}
	resp := &agentcomposev2.ListProjectsResponse{
		TotalCount: uint32(result.TotalCount),
		HasMore:    result.HasMore,
		NextOffset: uint32(result.NextOffset),
	}
	for _, project := range result.Projects {
		resp.Projects = append(resp.Projects, projectSummaryResponse(project, nil, nil))
	}
	return connect.NewResponse(resp), nil
}

func (p *ProjectService) RemoveProject(ctx context.Context, req *connect.Request[agentcomposev2.RemoveProjectRequest]) (*connect.Response[agentcomposev2.RemoveProjectResponse], error) {
	result, err := p.projectQueryUsecase().RemoveProject(ctx, projectdomain.RemoveProjectRequest{
		Ref:           projectRefFromProto(req.Msg.GetProject()),
		RemoveHistory: req.Msg.GetRemoveHistory(),
	})
	if err != nil {
		return nil, connect.NewError(projectQueryConnectCode(err), err)
	}
	return connect.NewResponse(&agentcomposev2.RemoveProjectResponse{
		Project: projectResponse(result.Project, nil, result.Agents, result.Schedulers),
		Changes: projectChangesToProto(result.Changes),
	}), nil
}

func (p *ProjectService) projectQueryUsecase() *projectdomain.QueryUsecase {
	return projectdomain.NewQueryUsecase(projectdomain.QueryUsecaseOptions{
		Store:           p.service.configDB,
		Runtime:         projectRuntimeAdapter{service: p.service},
		StableProjectID: StableProjectID,
	})
}

func projectRefFromProto(ref *agentcomposev2.ProjectRef) projectdomain.ProjectRef {
	if ref == nil {
		return projectdomain.ProjectRef{}
	}
	return projectdomain.ProjectRef{
		ProjectID:  ref.GetProjectId(),
		Name:       ref.GetName(),
		SourcePath: ref.GetSourcePath(),
	}
}

func projectQueryConnectCode(err error) connect.Code {
	switch projectdomain.ErrorKindOf(err) {
	case projectdomain.ErrorKindValidation:
		return connect.CodeInvalidArgument
	case projectdomain.ErrorKindNotFound:
		return connect.CodeNotFound
	case projectdomain.ErrorKindUnimplemented:
		return connect.CodeUnimplemented
	case projectdomain.ErrorKindRuntime, projectdomain.ErrorKindStorage, projectdomain.ErrorKindUnknown:
		return connect.CodeInternal
	default:
		return connect.CodeInternal
	}
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

func projectChangesToProto(changes []projectdomain.Change) []*agentcomposev2.ProjectChange {
	if len(changes) == 0 {
		return nil
	}
	result := make([]*agentcomposev2.ProjectChange, 0, len(changes))
	for _, change := range changes {
		result = append(result, &agentcomposev2.ProjectChange{
			Action:       projectChangeActionToProto(change.Kind),
			ResourceType: change.Resource,
			ResourceId:   change.ID,
			Name:         change.Name,
			Message:      change.Message,
		})
	}
	return result
}

func projectChangeActionToProto(kind projectdomain.ChangeKind) agentcomposev2.ProjectChangeAction {
	switch kind {
	case projectdomain.ChangeKindCreated:
		return agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_CREATED
	case projectdomain.ChangeKindUpdated:
		return agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_UPDATED
	case projectdomain.ChangeKindDeleted:
		return agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_REMOVED
	case projectdomain.ChangeKindUnchanged:
		return agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_UNCHANGED
	default:
		return agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_UNSPECIFIED
	}
}

func (s *ProjectService) WatchProject(ctx context.Context, req *connect.Request[agentcomposev2.WatchProjectRequest], stream *connect.ServerStream[agentcomposev2.WatchProjectResponse]) error {
	_, _, _ = ctx, req, stream
	return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("agentcompose.v2.ProjectService.WatchProject is not implemented"))
}
