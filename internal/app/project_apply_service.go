package app

import (
	"context"
	"fmt"

	projectdomain "agent-compose/internal/project"
	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

func newApplyProjectError(kind projectdomain.ErrorKind, err error) error {
	if err == nil {
		return projectdomain.NewError(kind, "", nil)
	}
	return projectdomain.NewError(kind, err.Error(), err)
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
	foundation projectdomain.ApplyResult
	project    *agentcomposev2.Project
	revision   *agentcomposev2.ProjectRevision
	changes    []*agentcomposev2.ProjectChange
	issues     []*agentcomposev2.ProjectValidationIssue
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
		Applied:   result.foundation.Applied,
		Unchanged: result.foundation.Unchanged,
	}
}

func (p *ProjectService) applyProjectWorkflow(ctx context.Context, req *agentcomposev2.ApplyProjectRequest) (*projectApplyResult, error) {
	var normalized normalizedV2Project
	var resources projectApplyResources
	var persisted projectApplyPersistResult
	var reconciled projectApplyReconcileResult
	var issueProtos []*agentcomposev2.ProjectValidationIssue
	var schedulerFailure *projectApplyResult

	usecase := projectdomain.NewApplyService(projectdomain.ApplyHooks{
		Normalize: func(context.Context) (projectdomain.ApplyResult, error) {
			var err error
			normalized, issueProtos, err = normalizeProjectServiceSpec(req.GetSpec(), req.GetSource(), req.GetExpectedSpecHash())
			if err != nil {
				return projectdomain.ApplyResult{}, err
			}
			return projectdomain.ApplyResult{
				ProjectName: normalizedProjectName(normalized),
				SpecHash:    normalized.specHash,
				Issues:      projectValidationIssuesFromProto(issueProtos),
			}, nil
		},
		CheckStore: func(context.Context, projectdomain.ApplyResult) error {
			if p.service.configDB == nil {
				return projectdomain.ApplyStoreRequiredError(normalizedProjectName(normalized))
			}
			return nil
		},
		Prepare: func(ctx context.Context, base projectdomain.ApplyResult, revision int64) (projectdomain.ApplyResult, error) {
			var err error
			resources, issueProtos, err = p.prepareProjectApplyResources(ctx, normalized, revision)
			if err != nil {
				return projectdomain.ApplyResult{}, err
			}
			result := projectdomain.ApplyResult{
				ProjectID:   resources.project.ID,
				ProjectName: base.ProjectName,
				SpecHash:    base.SpecHash,
				Issues:      projectValidationIssuesFromProto(issueProtos),
			}
			if len(issueProtos) == 0 {
				result.Changes = projectChangesFromProto(dryRunProjectChanges(resources.project, resources.agents, resources.agentDefinitions, resources.schedulers, resources.loaders))
			}
			return result, nil
		},
		EnsureRuntime: func(ctx context.Context, _ projectdomain.ApplyResult) error {
			if err := p.service.ensureProjectAgentImages(ctx, normalized.spec.Name, resources.agents); err != nil {
				return newApplyProjectError(projectdomain.ErrorKindRuntime, fmt.Errorf("apply project %s: %w", normalized.spec.Name, err))
			}
			return nil
		},
		Persist: func(ctx context.Context, result projectdomain.ApplyResult) (projectdomain.ApplyPersistResult, error) {
			var err error
			persisted, err = p.persistProjectApplyRevision(ctx, normalized, resources.project)
			if err != nil {
				return projectdomain.ApplyPersistResult{}, err
			}
			return projectdomain.ApplyPersistResult{
				Result: projectdomain.ApplyResult{
					ProjectID:   persisted.project.ID,
					ProjectName: result.ProjectName,
					Revision:    persisted.revision.Revision,
					SpecHash:    result.SpecHash,
					Changes:     projectChangesFromProto(projectApplyChanges(persisted.project, persisted.existingProject, persisted.projectFound, persisted.revision, persisted.revisionCreated)),
				},
				ProjectFound:     persisted.projectFound,
				RevisionCreated:  persisted.revisionCreated,
				ProjectUnchanged: projectRecordUnchanged(persisted.existingProject, persisted.project),
			}, nil
		},
		Reload: func(ctx context.Context, result projectdomain.ApplyResult, persistedResult projectdomain.ApplyPersistResult) (projectdomain.ApplyResult, error) {
			var err error
			resources, err = p.projectApplyResourcesForRevision(ctx, normalized, persisted.project, persisted.revision.Revision)
			if err != nil {
				return projectdomain.ApplyResult{}, err
			}
			return projectdomain.ApplyResult{
				ProjectID:   persisted.project.ID,
				ProjectName: result.ProjectName,
				Revision:    persisted.revision.Revision,
				SpecHash:    result.SpecHash,
				Changes:     result.Changes,
			}, nil
		},
		Reconcile: func(ctx context.Context, result projectdomain.ApplyResult, persistedResult projectdomain.ApplyPersistResult) (projectdomain.ApplyReconcileResult, error) {
			var err error
			reconciled, schedulerFailure, err = p.reconcileProjectApplyResources(ctx, normalized, persisted, resources)
			if err != nil {
				return projectdomain.ApplyReconcileResult{}, err
			}
			if schedulerFailure != nil {
				return projectdomain.ApplyReconcileResult{Failure: &schedulerFailure.foundation}, nil
			}
			return projectdomain.ApplyReconcileResult{
				Result: projectdomain.ApplyResult{
					ProjectID:   persisted.project.ID,
					ProjectName: result.ProjectName,
					Revision:    persisted.revision.Revision,
					SpecHash:    result.SpecHash,
					Changes:     projectChangesFromProto(reconciled.changes),
				},
				ResourcesUnchanged: reconciled.agentsUnchanged,
			}, nil
		},
	})

	foundation, err := usecase.Apply(ctx, projectdomain.ApplyRequest{DryRun: req.GetDryRun()})
	if err != nil {
		return nil, err
	}
	if schedulerFailure != nil {
		schedulerFailure.foundation = foundation
		return schedulerFailure, nil
	}
	if foundation.HasIssues() {
		return &projectApplyResult{
			foundation: foundation,
			issues:     issueProtos,
		}, nil
	}
	if foundation.DryRun {
		result := dryRunProjectApplyResult(normalized, resources)
		result.foundation = foundation
		return result, nil
	}
	return p.finalProjectApplyResult(ctx, normalized, persisted, resources, reconciled, foundation)
}

func (p *ProjectService) prepareProjectApplyResources(ctx context.Context, normalized normalizedV2Project, revision int64) (projectApplyResources, []*agentcomposev2.ProjectValidationIssue, error) {
	project, err := NewProjectRecordFromSpec(normalized.spec, normalized.sourcePath)
	if err != nil {
		return projectApplyResources{}, nil, newApplyProjectError(projectdomain.ErrorKindValidation, fmt.Errorf("apply project %s: %w", normalized.spec.Name, err))
	}
	if issues := p.validateProjectManagedAgentDefinitions(normalized); len(issues) > 0 {
		return projectApplyResources{}, issues, nil
	}
	if issues := p.validateProjectManagedSchedulers(ctx, normalized); len(issues) > 0 {
		return projectApplyResources{}, issues, nil
	}
	resources, err := p.projectApplyResourcesForRevision(ctx, normalized, project, revision)
	if err != nil {
		return projectApplyResources{}, nil, err
	}
	return resources, nil, nil
}

func (p *ProjectService) projectApplyResourcesForRevision(ctx context.Context, normalized normalizedV2Project, project ProjectRecord, revision int64) (projectApplyResources, error) {
	agentRecords, err := projectAgentRecordsFromSpec(project.ID, revision, normalized.spec)
	if err != nil {
		return projectApplyResources{}, newApplyProjectError(projectdomain.ErrorKindValidation, fmt.Errorf("apply project %s: %w", normalized.spec.Name, err))
	}
	agentDefinitions, err := projectManagedAgentDefinitionsFromSpec(project, revision, normalized.spec)
	if err != nil {
		return projectApplyResources{}, newApplyProjectError(projectdomain.ErrorKindValidation, fmt.Errorf("apply project %s: %w", normalized.spec.Name, err))
	}
	schedulerRecords, managedLoaders, err := p.projectManagedSchedulersFromSpec(ctx, project, revision, normalized.spec)
	if err != nil {
		return projectApplyResources{}, newApplyProjectError(projectdomain.ErrorKindValidation, fmt.Errorf("apply project %s: %w", normalized.spec.Name, err))
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
	changes := dryRunProjectChanges(resources.project, resources.agents, resources.agentDefinitions, resources.schedulers, resources.loaders)
	revision := projectRevisionResponse(ProjectRevisionRecord{ProjectID: resources.project.ID, SpecHash: normalized.specHash}, normalized.specProto)
	return &projectApplyResult{
		foundation: projectdomain.ApplyResult{
			ProjectID:   resources.project.ID,
			ProjectName: normalizedProjectName(normalized),
			Revision:    int64(revision.GetRevision()),
			SpecHash:    normalized.specHash,
			DryRun:      true,
			Applied:     false,
			Unchanged:   false,
			Changes:     projectChangesFromProto(changes),
		},
		project:  projectResponse(resources.project, normalized.specProto, resources.agents, resources.schedulers),
		revision: revision,
		changes:  changes,
	}
}

func projectApplyResultFromIssues(normalized normalizedV2Project, issues []*agentcomposev2.ProjectValidationIssue) *projectApplyResult {
	return &projectApplyResult{
		foundation: projectdomain.ApplyResult{
			ProjectName: normalizedProjectName(normalized),
			SpecHash:    normalized.specHash,
			Issues:      projectValidationIssuesFromProto(issues),
		},
		issues: issues,
	}
}

func (p *ProjectService) persistProjectApplyRevision(ctx context.Context, normalized normalizedV2Project, project ProjectRecord) (projectApplyPersistResult, error) {
	existingProject, projectFound, err := p.service.configDB.GetProjectIncludingRemoved(ctx, project.ID)
	if err != nil {
		return projectApplyPersistResult{}, newApplyProjectError(projectdomain.ErrorKindStorage, fmt.Errorf("apply project %s: load existing project: %w", normalized.spec.Name, err))
	}
	project, err = p.service.configDB.UpsertProject(ctx, project)
	if err != nil {
		return projectApplyPersistResult{}, newApplyProjectError(projectdomain.ErrorKindStorage, fmt.Errorf("apply project %s: upsert project: %w", normalized.spec.Name, err))
	}
	specJSON, err := normalized.spec.MarshalCanonicalJSON(false)
	if err != nil {
		return projectApplyPersistResult{}, newApplyProjectError(projectdomain.ErrorKindUnknown, fmt.Errorf("apply project %s: marshal project spec: %w", normalized.spec.Name, err))
	}
	revision, revisionCreated, err := p.service.configDB.SaveProjectRevision(ctx, ProjectRevisionRecord{
		ProjectID: project.ID,
		SpecHash:  normalized.specHash,
		SpecJSON:  string(specJSON),
	})
	if err != nil {
		return projectApplyPersistResult{}, newApplyProjectError(projectdomain.ErrorKindStorage, fmt.Errorf("apply project %s: save revision: %w", normalized.spec.Name, err))
	}
	project, err = p.service.configDB.GetProject(ctx, project.ID)
	if err != nil {
		return projectApplyPersistResult{}, newApplyProjectError(projectdomain.ErrorKindStorage, fmt.Errorf("apply project %s: reload project: %w", normalized.spec.Name, err))
	}
	return projectApplyPersistResult{
		project:         project,
		existingProject: existingProject,
		projectFound:    projectFound,
		revision:        revision,
		revisionCreated: revisionCreated,
	}, nil
}

func (p *ProjectService) reconcileProjectApplyResources(ctx context.Context, normalized normalizedV2Project, persisted projectApplyPersistResult, resources projectApplyResources) (projectApplyReconcileResult, *projectApplyResult, error) {
	changes := projectApplyChanges(persisted.project, persisted.existingProject, persisted.projectFound, persisted.revision, persisted.revisionCreated)
	agentChanges, agentsUnchanged, err := p.reconcileProjectApplyAgents(ctx, normalized, persisted.project, resources.agents, resources.agentDefinitions)
	if err != nil {
		return projectApplyReconcileResult{}, nil, err
	}
	changes = append(changes, agentChanges...)

	schedulerChanges, schedulersUnchanged, err := p.reconcileProjectManagedSchedulers(ctx, persisted.project, resources.schedulers, resources.loaders)
	if err != nil {
		changes = append(changes, schedulerChanges...)
		result, failureErr := p.schedulerReconcileFailureProjectApplyResult(ctx, normalized, persisted, changes, err)
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

func (p *ProjectService) reconcileProjectApplyAgents(ctx context.Context, normalized normalizedV2Project, project ProjectRecord, agents []ProjectAgentRecord, agentDefinitions []AgentDefinition) ([]*agentcomposev2.ProjectChange, bool, error) {
	changes := make([]*agentcomposev2.ProjectChange, 0, len(agents)+len(agentDefinitions))
	agentsUnchanged := true
	for _, agent := range agents {
		existingAgent, found, err := getProjectAgentIfExists(ctx, p.service.configDB, project.ID, agent.AgentName)
		if err != nil {
			return nil, false, newApplyProjectError(projectdomain.ErrorKindStorage, fmt.Errorf("apply project %s: load agent %s: %w", normalized.spec.Name, agent.AgentName, err))
		}
		if _, err := p.service.configDB.UpsertProjectAgent(ctx, agent); err != nil {
			return nil, false, newApplyProjectError(projectdomain.ErrorKindStorage, fmt.Errorf("apply project %s: upsert agent %s: %w", normalized.spec.Name, agent.AgentName, err))
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
	agentDefinitionChanges, agentDefinitionsUnchanged, err := p.reconcileProjectManagedAgentDefinitions(ctx, project, agentDefinitions)
	if err != nil {
		return nil, false, newApplyProjectError(projectdomain.ErrorKindStorage, fmt.Errorf("apply project %s: %w", normalized.spec.Name, err))
	}
	if !agentDefinitionsUnchanged {
		agentsUnchanged = false
	}
	changes = append(changes, agentDefinitionChanges...)
	return changes, agentsUnchanged, nil
}

func (p *ProjectService) schedulerReconcileFailureProjectApplyResult(ctx context.Context, normalized normalizedV2Project, persisted projectApplyPersistResult, changes []*agentcomposev2.ProjectChange, reconcileErr error) (*projectApplyResult, error) {
	agents, listAgentsErr := p.service.configDB.ListProjectAgents(ctx, persisted.project.ID)
	if listAgentsErr != nil {
		return nil, newApplyProjectError(projectdomain.ErrorKindStorage, fmt.Errorf("apply project %s: %w; list project agents after reconcile failure: %v", normalized.spec.Name, reconcileErr, listAgentsErr))
	}
	schedulers, listSchedulersErr := p.service.configDB.ListProjectSchedulers(ctx, persisted.project.ID)
	if listSchedulersErr != nil {
		return nil, newApplyProjectError(projectdomain.ErrorKindStorage, fmt.Errorf("apply project %s: %w; list project schedulers after reconcile failure: %v", normalized.spec.Name, reconcileErr, listSchedulersErr))
	}
	issues := []*agentcomposev2.ProjectValidationIssue{
		projectValidationIssue("reconcile.schedulers", fmt.Sprintf("apply project %s: %v", normalized.spec.Name, reconcileErr)),
	}
	return &projectApplyResult{
		foundation: projectdomain.ApplyResult{
			ProjectID:   persisted.project.ID,
			ProjectName: normalizedProjectName(normalized),
			Revision:    persisted.revision.Revision,
			SpecHash:    normalized.specHash,
			Applied:     false,
			Unchanged:   false,
			Changes:     projectChangesFromProto(changes),
			Issues:      projectValidationIssuesFromProto(issues),
		},
		project:  projectResponse(persisted.project, normalized.specProto, agents, schedulers),
		revision: projectRevisionResponse(persisted.revision, normalized.specProto),
		changes:  changes,
		issues:   issues,
	}, nil
}

func (p *ProjectService) finalProjectApplyResult(ctx context.Context, normalized normalizedV2Project, persisted projectApplyPersistResult, resources projectApplyResources, reconciled projectApplyReconcileResult, foundation projectdomain.ApplyResult) (*projectApplyResult, error) {
	agents, err := p.service.configDB.ListProjectAgents(ctx, persisted.project.ID)
	if err != nil {
		return nil, newApplyProjectError(projectdomain.ErrorKindStorage, fmt.Errorf("apply project %s: list project agents: %w", normalized.spec.Name, err))
	}
	schedulers, err := p.service.configDB.ListProjectSchedulers(ctx, persisted.project.ID)
	if err != nil {
		return nil, newApplyProjectError(projectdomain.ErrorKindStorage, fmt.Errorf("apply project %s: list project schedulers: %w", normalized.spec.Name, err))
	}
	return &projectApplyResult{
		foundation: foundation,
		project:    projectResponse(resources.project, normalized.specProto, agents, schedulers),
		revision:   projectRevisionResponse(persisted.revision, normalized.specProto),
		changes:    reconciled.changes,
	}, nil
}

func normalizedProjectName(normalized normalizedV2Project) string {
	if normalized.spec == nil {
		return ""
	}
	return normalized.spec.Name
}

func projectChangesFromProto(changes []*agentcomposev2.ProjectChange) []projectdomain.Change {
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
			Message:  change.GetMessage(),
		})
	}
	return result
}

func projectChangeKindFromProto(action agentcomposev2.ProjectChangeAction) projectdomain.ChangeKind {
	switch action {
	case agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_CREATED:
		return projectdomain.ChangeKindCreated
	case agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_UPDATED:
		return projectdomain.ChangeKindUpdated
	case agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_REMOVED:
		return projectdomain.ChangeKindDeleted
	case agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_UNCHANGED:
		return projectdomain.ChangeKindUnchanged
	default:
		return ""
	}
}

func projectValidationIssuesFromProto(issues []*agentcomposev2.ProjectValidationIssue) []projectdomain.ValidationIssue {
	if len(issues) == 0 {
		return nil
	}
	result := make([]projectdomain.ValidationIssue, 0, len(issues))
	for _, issue := range issues {
		if issue == nil {
			continue
		}
		result = append(result, projectdomain.ValidationIssue{
			Field:   issue.GetPath(),
			Message: issue.GetMessage(),
		})
	}
	return result
}
