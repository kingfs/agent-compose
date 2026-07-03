package app

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

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
	if p.service.configDB == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("config store is required"))
	}
	project, err := p.resolveProjectRef(ctx, req.Msg.GetProject())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "ambiguous") {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	agents, err := p.service.configDB.ListProjectAgents(ctx, project.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	schedulers, err := p.service.configDB.ListProjectSchedulers(ctx, project.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	var spec *agentcomposev2.ProjectSpec
	if req.Msg.GetIncludeSpec() && project.CurrentRevision > 0 {
		revision, err := p.service.configDB.GetProjectRevision(ctx, project.ID, project.CurrentRevision)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, err)
		}
		spec, err = decodeProjectRevisionSpec(revision.SpecJSON)
		if err != nil {
			return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("decode project %s revision %d: %w", project.Name, project.CurrentRevision, err))
		}
	}
	return connect.NewResponse(&agentcomposev2.GetProjectResponse{
		Project: projectResponse(project, spec, agents, schedulers),
	}), nil
}

func (p *ProjectService) ListProjects(ctx context.Context, req *connect.Request[agentcomposev2.ListProjectsRequest]) (*connect.Response[agentcomposev2.ListProjectsResponse], error) {
	if p.service.configDB == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("config store is required"))
	}
	result, err := p.service.configDB.ListProjects(ctx, ProjectListOptions{
		Query:          req.Msg.GetQuery(),
		IncludeRemoved: req.Msg.GetIncludeRemoved(),
		Offset:         int(req.Msg.GetOffset()),
		Limit:          int(req.Msg.GetLimit()),
	})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
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
	if p.service.configDB == nil {
		return nil, connect.NewError(connect.CodeInternal, fmt.Errorf("config store is required"))
	}
	if req.Msg.GetRemoveHistory() {
		return nil, connect.NewError(connect.CodeUnimplemented, fmt.Errorf("project history removal is not implemented"))
	}
	project, err := p.resolveProjectRef(ctx, req.Msg.GetProject())
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, connect.NewError(connect.CodeNotFound, err)
		}
		if strings.Contains(err.Error(), "required") || strings.Contains(err.Error(), "ambiguous") {
			return nil, connect.NewError(connect.CodeInvalidArgument, err)
		}
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	changes, err := p.service.downProject(ctx, project)
	if err != nil {
		return nil, err
	}
	agents, err := p.service.configDB.ListProjectAgents(ctx, project.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	schedulers, err := p.service.configDB.ListProjectSchedulers(ctx, project.ID)
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	return connect.NewResponse(&agentcomposev2.RemoveProjectResponse{
		Project: projectResponse(project, nil, agents, schedulers),
		Changes: changes,
	}), nil
}

func (s *ProjectService) WatchProject(ctx context.Context, req *connect.Request[agentcomposev2.WatchProjectRequest], stream *connect.ServerStream[agentcomposev2.WatchProjectResponse]) error {
	_, _, _ = ctx, req, stream
	return connect.NewError(connect.CodeUnimplemented, fmt.Errorf("agentcompose.v2.ProjectService.WatchProject is not implemented"))
}
