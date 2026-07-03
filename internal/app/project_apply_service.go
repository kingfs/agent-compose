package app

import (
	"context"
	"fmt"

	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

type projectApplyErrorCode int

const (
	projectApplyErrorCodeInternal projectApplyErrorCode = iota
	projectApplyErrorCodeInvalidArgument
	projectApplyErrorCodeUnavailable
)

type projectApplyError struct {
	code projectApplyErrorCode
	err  error
}

func (e projectApplyError) Error() string {
	if e.err == nil {
		return ""
	}
	return e.err.Error()
}

func (e projectApplyError) Unwrap() error {
	return e.err
}

func newProjectApplyError(code projectApplyErrorCode, err error) error {
	return projectApplyError{code: code, err: err}
}

type projectApplyResources struct {
	project          ProjectRecord
	agents           []ProjectAgentRecord
	agentDefinitions []AgentDefinition
	schedulers       []ProjectSchedulerRecord
	loaders          []Loader
}

type projectApplyPersistResult struct {
	project         ProjectRecord
	existingProject ProjectRecord
	projectFound    bool
	revision        ProjectRevisionRecord
	revisionCreated bool
}

type projectApplyReconcileResult struct {
	changes         []*agentcomposev2.ProjectChange
	agentsUnchanged bool
}

type projectApplyResult struct {
	project   *agentcomposev2.Project
	revision  *agentcomposev2.ProjectRevision
	changes   []*agentcomposev2.ProjectChange
	issues    []*agentcomposev2.ProjectValidationIssue
	applied   bool
	unchanged bool
}

func projectApplyResponseFromResult(result *projectApplyResult) *agentcomposev2.ApplyProjectResponse {
	if result == nil {
		return &agentcomposev2.ApplyProjectResponse{}
	}
	return &agentcomposev2.ApplyProjectResponse{
		Project:   result.project,
		Revision:  result.revision,
		Changes:   result.changes,
		Issues:    result.issues,
		Applied:   result.applied,
		Unchanged: result.unchanged,
	}
}

func (s *Service) applyProjectWorkflow(ctx context.Context, req *agentcomposev2.ApplyProjectRequest) (*projectApplyResult, error) {
	normalized, issues, err := normalizeProjectServiceSpec(req.GetSpec(), req.GetSource(), req.GetExpectedSpecHash())
	if err != nil {
		return nil, newProjectApplyError(projectApplyErrorCodeInternal, err)
	}
	if len(issues) > 0 {
		return &projectApplyResult{issues: issues}, nil
	}
	if s.configDB == nil {
		return nil, newProjectApplyError(projectApplyErrorCodeInternal, fmt.Errorf("apply project %s: config store is required", normalized.spec.Name))
	}

	resources, issues, err := s.prepareProjectApplyResources(ctx, normalized, 0)
	if err != nil {
		return nil, err
	}
	if len(issues) > 0 {
		return &projectApplyResult{issues: issues}, nil
	}
	if req.GetDryRun() {
		return dryRunProjectApplyResult(normalized, resources), nil
	}
	if err := s.ensureProjectAgentImages(ctx, normalized.spec.Name, resources.agents); err != nil {
		return nil, newProjectApplyError(projectApplyErrorCodeUnavailable, fmt.Errorf("apply project %s: %w", normalized.spec.Name, err))
	}

	persisted, err := s.persistProjectApplyRevision(ctx, normalized, resources.project)
	if err != nil {
		return nil, err
	}
	resources, err = s.projectApplyResourcesForRevision(ctx, normalized, persisted.project, persisted.revision.Revision)
	if err != nil {
		return nil, err
	}
	reconciled, schedulerFailure, err := s.reconcileProjectApplyResources(ctx, normalized, persisted, resources)
	if err != nil {
		return nil, err
	}
	if schedulerFailure != nil {
		return schedulerFailure, nil
	}
	return s.finalProjectApplyResult(ctx, normalized, persisted, resources, reconciled)
}

func (s *Service) prepareProjectApplyResources(ctx context.Context, normalized normalizedV2Project, revision int64) (projectApplyResources, []*agentcomposev2.ProjectValidationIssue, error) {
	project, err := NewProjectRecordFromSpec(normalized.spec, normalized.sourcePath)
	if err != nil {
		return projectApplyResources{}, nil, newProjectApplyError(projectApplyErrorCodeInvalidArgument, fmt.Errorf("apply project %s: %w", normalized.spec.Name, err))
	}
	if issues := s.validateProjectManagedAgentDefinitions(normalized); len(issues) > 0 {
		return projectApplyResources{}, issues, nil
	}
	if issues := s.validateProjectManagedSchedulers(ctx, normalized); len(issues) > 0 {
		return projectApplyResources{}, issues, nil
	}
	resources, err := s.projectApplyResourcesForRevision(ctx, normalized, project, revision)
	if err != nil {
		return projectApplyResources{}, nil, err
	}
	return resources, nil, nil
}

func (s *Service) projectApplyResourcesForRevision(ctx context.Context, normalized normalizedV2Project, project ProjectRecord, revision int64) (projectApplyResources, error) {
	agentRecords, err := projectAgentRecordsFromSpec(project.ID, revision, normalized.spec)
	if err != nil {
		return projectApplyResources{}, newProjectApplyError(projectApplyErrorCodeInvalidArgument, fmt.Errorf("apply project %s: %w", normalized.spec.Name, err))
	}
	agentDefinitions, err := projectManagedAgentDefinitionsFromSpec(project, revision, normalized.spec)
	if err != nil {
		return projectApplyResources{}, newProjectApplyError(projectApplyErrorCodeInvalidArgument, fmt.Errorf("apply project %s: %w", normalized.spec.Name, err))
	}
	schedulerRecords, managedLoaders, err := s.projectManagedSchedulersFromSpec(ctx, project, revision, normalized.spec)
	if err != nil {
		return projectApplyResources{}, newProjectApplyError(projectApplyErrorCodeInvalidArgument, fmt.Errorf("apply project %s: %w", normalized.spec.Name, err))
	}
	return projectApplyResources{
		project:          project,
		agents:           agentRecords,
		agentDefinitions: agentDefinitions,
		schedulers:       schedulerRecords,
		loaders:          managedLoaders,
	}, nil
}

func dryRunProjectApplyResult(normalized normalizedV2Project, resources projectApplyResources) *projectApplyResult {
	return &projectApplyResult{
		project:  projectResponse(resources.project, normalized.specProto, resources.agents, resources.schedulers),
		revision: projectRevisionResponse(ProjectRevisionRecord{ProjectID: resources.project.ID, SpecHash: normalized.specHash}, normalized.specProto),
		changes:  dryRunProjectChanges(resources.project, resources.agents, resources.agentDefinitions, resources.schedulers, resources.loaders),
		applied:  false,
	}
}

func (s *Service) persistProjectApplyRevision(ctx context.Context, normalized normalizedV2Project, project ProjectRecord) (projectApplyPersistResult, error) {
	existingProject, projectFound, err := s.configDB.GetProjectIncludingRemoved(ctx, project.ID)
	if err != nil {
		return projectApplyPersistResult{}, newProjectApplyError(projectApplyErrorCodeInternal, fmt.Errorf("apply project %s: load existing project: %w", normalized.spec.Name, err))
	}
	project, err = s.configDB.UpsertProject(ctx, project)
	if err != nil {
		return projectApplyPersistResult{}, newProjectApplyError(projectApplyErrorCodeInternal, fmt.Errorf("apply project %s: upsert project: %w", normalized.spec.Name, err))
	}
	specJSON, err := normalized.spec.MarshalCanonicalJSON(false)
	if err != nil {
		return projectApplyPersistResult{}, newProjectApplyError(projectApplyErrorCodeInternal, fmt.Errorf("apply project %s: marshal project spec: %w", normalized.spec.Name, err))
	}
	revision, revisionCreated, err := s.configDB.SaveProjectRevision(ctx, ProjectRevisionRecord{
		ProjectID: project.ID,
		SpecHash:  normalized.specHash,
		SpecJSON:  string(specJSON),
	})
	if err != nil {
		return projectApplyPersistResult{}, newProjectApplyError(projectApplyErrorCodeInternal, fmt.Errorf("apply project %s: save revision: %w", normalized.spec.Name, err))
	}
	project, err = s.configDB.GetProject(ctx, project.ID)
	if err != nil {
		return projectApplyPersistResult{}, newProjectApplyError(projectApplyErrorCodeInternal, fmt.Errorf("apply project %s: reload project: %w", normalized.spec.Name, err))
	}
	return projectApplyPersistResult{
		project:         project,
		existingProject: existingProject,
		projectFound:    projectFound,
		revision:        revision,
		revisionCreated: revisionCreated,
	}, nil
}

func (s *Service) reconcileProjectApplyResources(ctx context.Context, normalized normalizedV2Project, persisted projectApplyPersistResult, resources projectApplyResources) (projectApplyReconcileResult, *projectApplyResult, error) {
	changes := projectApplyChanges(persisted.project, persisted.existingProject, persisted.projectFound, persisted.revision, persisted.revisionCreated)
	agentChanges, agentsUnchanged, err := s.reconcileProjectApplyAgents(ctx, normalized, persisted.project, resources.agents, resources.agentDefinitions)
	if err != nil {
		return projectApplyReconcileResult{}, nil, err
	}
	changes = append(changes, agentChanges...)

	schedulerChanges, schedulersUnchanged, err := s.reconcileProjectManagedSchedulers(ctx, persisted.project, resources.schedulers, resources.loaders)
	if err != nil {
		changes = append(changes, schedulerChanges...)
		result, failureErr := s.schedulerReconcileFailureProjectApplyResult(ctx, normalized, persisted, changes, err)
		if failureErr != nil {
			return projectApplyReconcileResult{}, nil, failureErr
		}
		return projectApplyReconcileResult{}, result, nil
	}
	if !schedulersUnchanged {
		agentsUnchanged = false
	}
	changes = append(changes, schedulerChanges...)
	return projectApplyReconcileResult{
		changes:         changes,
		agentsUnchanged: agentsUnchanged,
	}, nil, nil
}

func (s *Service) reconcileProjectApplyAgents(ctx context.Context, normalized normalizedV2Project, project ProjectRecord, agents []ProjectAgentRecord, agentDefinitions []AgentDefinition) ([]*agentcomposev2.ProjectChange, bool, error) {
	changes := make([]*agentcomposev2.ProjectChange, 0, len(agents)+len(agentDefinitions))
	agentsUnchanged := true
	for _, agent := range agents {
		existingAgent, found, err := getProjectAgentIfExists(ctx, s.configDB, project.ID, agent.AgentName)
		if err != nil {
			return nil, false, newProjectApplyError(projectApplyErrorCodeInternal, fmt.Errorf("apply project %s: load agent %s: %w", normalized.spec.Name, agent.AgentName, err))
		}
		if _, err := s.configDB.UpsertProjectAgent(ctx, agent); err != nil {
			return nil, false, newProjectApplyError(projectApplyErrorCodeInternal, fmt.Errorf("apply project %s: upsert agent %s: %w", normalized.spec.Name, agent.AgentName, err))
		}
		action := agentChangeAction(existingAgent, found, agent)
		if action != agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_UNCHANGED {
			agentsUnchanged = false
		}
		changes = append(changes, &agentcomposev2.ProjectChange{
			Action:       action,
			ResourceType: "project_agent",
			ResourceId:   agent.ManagedAgentID,
			Name:         agent.AgentName,
		})
	}
	agentDefinitionChanges, agentDefinitionsUnchanged, err := s.reconcileProjectManagedAgentDefinitions(ctx, project, agentDefinitions)
	if err != nil {
		return nil, false, newProjectApplyError(projectApplyErrorCodeInternal, fmt.Errorf("apply project %s: %w", normalized.spec.Name, err))
	}
	if !agentDefinitionsUnchanged {
		agentsUnchanged = false
	}
	changes = append(changes, agentDefinitionChanges...)
	return changes, agentsUnchanged, nil
}

func (s *Service) schedulerReconcileFailureProjectApplyResult(ctx context.Context, normalized normalizedV2Project, persisted projectApplyPersistResult, changes []*agentcomposev2.ProjectChange, reconcileErr error) (*projectApplyResult, error) {
	agents, listAgentsErr := s.configDB.ListProjectAgents(ctx, persisted.project.ID)
	if listAgentsErr != nil {
		return nil, newProjectApplyError(projectApplyErrorCodeInternal, fmt.Errorf("apply project %s: %w; list project agents after reconcile failure: %v", normalized.spec.Name, reconcileErr, listAgentsErr))
	}
	schedulers, listSchedulersErr := s.configDB.ListProjectSchedulers(ctx, persisted.project.ID)
	if listSchedulersErr != nil {
		return nil, newProjectApplyError(projectApplyErrorCodeInternal, fmt.Errorf("apply project %s: %w; list project schedulers after reconcile failure: %v", normalized.spec.Name, reconcileErr, listSchedulersErr))
	}
	return &projectApplyResult{
		project:  projectResponse(persisted.project, normalized.specProto, agents, schedulers),
		revision: projectRevisionResponse(persisted.revision, normalized.specProto),
		changes:  changes,
		issues: []*agentcomposev2.ProjectValidationIssue{
			projectValidationIssue("reconcile.schedulers", fmt.Sprintf("apply project %s: %v", normalized.spec.Name, reconcileErr)),
		},
		applied:   false,
		unchanged: false,
	}, nil
}

func (s *Service) finalProjectApplyResult(ctx context.Context, normalized normalizedV2Project, persisted projectApplyPersistResult, resources projectApplyResources, reconciled projectApplyReconcileResult) (*projectApplyResult, error) {
	agents, err := s.configDB.ListProjectAgents(ctx, persisted.project.ID)
	if err != nil {
		return nil, newProjectApplyError(projectApplyErrorCodeInternal, fmt.Errorf("apply project %s: list project agents: %w", normalized.spec.Name, err))
	}
	schedulers, err := s.configDB.ListProjectSchedulers(ctx, persisted.project.ID)
	if err != nil {
		return nil, newProjectApplyError(projectApplyErrorCodeInternal, fmt.Errorf("apply project %s: list project schedulers: %w", normalized.spec.Name, err))
	}
	return &projectApplyResult{
		project:  projectResponse(resources.project, normalized.specProto, agents, schedulers),
		revision: projectRevisionResponse(persisted.revision, normalized.specProto),
		changes:  reconciled.changes,
		applied:  true,
		unchanged: persisted.projectFound &&
			!persisted.revisionCreated &&
			projectRecordUnchanged(persisted.existingProject, persisted.project) &&
			reconciled.agentsUnchanged,
	}, nil
}
