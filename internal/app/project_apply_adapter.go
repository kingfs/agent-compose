package app

import (
	"context"
	"fmt"

	projectdomain "agent-compose/internal/project"
	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

type projectApplyAdapter struct {
	service *ProjectService
	req     *agentcomposev2.ApplyProjectRequest

	normalized       normalizedV2Project
	resources        projectApplyResources
	persisted        projectApplyPersistResult
	reconciled       projectApplyReconcileResult
	issueProtos      []*agentcomposev2.ProjectValidationIssue
	schedulerFailure *projectApplyResult
}

func newProjectApplyAdapter(service *ProjectService, req *agentcomposev2.ApplyProjectRequest) *projectApplyAdapter {
	return &projectApplyAdapter{
		service: service,
		req:     req,
	}
}

func (a *projectApplyAdapter) hooks() projectdomain.ApplyHooks {
	return projectdomain.ApplyHooks{
		Normalize:     a.normalize,
		CheckStore:    a.checkStore,
		Prepare:       a.prepare,
		EnsureRuntime: a.ensureRuntime,
		Persist:       a.persist,
		Reload:        a.reload,
		Reconcile:     a.reconcile,
	}
}

func (a *projectApplyAdapter) normalize(context.Context) (projectdomain.ApplyResult, error) {
	var err error
	a.normalized, a.issueProtos, err = normalizeProjectServiceSpec(a.req.GetSpec(), a.req.GetSource(), a.req.GetExpectedSpecHash())
	if err != nil {
		return projectdomain.ApplyResult{}, err
	}
	return projectdomain.ApplyResult{
		ProjectName: normalizedProjectName(a.normalized),
		SpecHash:    a.normalized.specHash,
		Issues:      projectValidationIssuesFromProto(a.issueProtos),
	}, nil
}

func (a *projectApplyAdapter) checkStore(context.Context, projectdomain.ApplyResult) error {
	if a.service.service.configDB == nil {
		return projectdomain.ApplyStoreRequiredError(normalizedProjectName(a.normalized))
	}
	return nil
}

func (a *projectApplyAdapter) prepare(ctx context.Context, base projectdomain.ApplyResult, revision int64) (projectdomain.ApplyResult, error) {
	var err error
	a.resources, a.issueProtos, err = a.service.prepareProjectApplyResources(ctx, a.normalized, revision)
	if err != nil {
		return projectdomain.ApplyResult{}, err
	}
	result := projectdomain.ApplyResult{
		ProjectID:   a.resources.project.ID,
		ProjectName: base.ProjectName,
		SpecHash:    base.SpecHash,
		Issues:      projectValidationIssuesFromProto(a.issueProtos),
	}
	if len(a.issueProtos) == 0 {
		result.Changes = projectChangesFromProto(dryRunProjectChanges(a.resources.project, a.resources.agents, a.resources.agentDefinitions, a.resources.schedulers, a.resources.loaders))
	}
	return result, nil
}

func (a *projectApplyAdapter) ensureRuntime(ctx context.Context, _ projectdomain.ApplyResult) error {
	if err := a.service.service.ensureProjectAgentImages(ctx, a.normalized.spec.Name, a.resources.agents); err != nil {
		return newApplyProjectError(projectdomain.ErrorKindRuntime, fmt.Errorf("apply project %s: %w", a.normalized.spec.Name, err))
	}
	return nil
}

func (a *projectApplyAdapter) persist(ctx context.Context, result projectdomain.ApplyResult) (projectdomain.ApplyPersistResult, error) {
	var err error
	a.persisted, err = a.service.persistProjectApplyRevision(ctx, a.normalized, a.resources.project)
	if err != nil {
		return projectdomain.ApplyPersistResult{}, err
	}
	return projectdomain.ApplyPersistResult{
		Result: projectdomain.ApplyResult{
			ProjectID:   a.persisted.project.ID,
			ProjectName: result.ProjectName,
			Revision:    a.persisted.revision.Revision,
			SpecHash:    result.SpecHash,
			Changes:     projectChangesFromProto(projectApplyChanges(a.persisted.project, a.persisted.existingProject, a.persisted.projectFound, a.persisted.revision, a.persisted.revisionCreated)),
		},
		ProjectFound:     a.persisted.projectFound,
		RevisionCreated:  a.persisted.revisionCreated,
		ProjectUnchanged: projectRecordUnchanged(a.persisted.existingProject, a.persisted.project),
	}, nil
}

func (a *projectApplyAdapter) reload(ctx context.Context, result projectdomain.ApplyResult, _ projectdomain.ApplyPersistResult) (projectdomain.ApplyResult, error) {
	var err error
	a.resources, err = a.service.projectApplyResourcesForRevision(ctx, a.normalized, a.persisted.project, a.persisted.revision.Revision)
	if err != nil {
		return projectdomain.ApplyResult{}, err
	}
	return projectdomain.ApplyResult{
		ProjectID:   a.persisted.project.ID,
		ProjectName: result.ProjectName,
		Revision:    a.persisted.revision.Revision,
		SpecHash:    result.SpecHash,
		Changes:     result.Changes,
	}, nil
}

func (a *projectApplyAdapter) reconcile(ctx context.Context, result projectdomain.ApplyResult, _ projectdomain.ApplyPersistResult) (projectdomain.ApplyReconcileResult, error) {
	var err error
	a.reconciled, a.schedulerFailure, err = a.service.reconcileProjectApplyResources(ctx, a.normalized, a.persisted, a.resources)
	if err != nil {
		return projectdomain.ApplyReconcileResult{}, err
	}
	if a.schedulerFailure != nil {
		return projectdomain.ApplyReconcileResult{Failure: &a.schedulerFailure.foundation}, nil
	}
	return projectdomain.ApplyReconcileResult{
		Result: projectdomain.ApplyResult{
			ProjectID:   a.persisted.project.ID,
			ProjectName: result.ProjectName,
			Revision:    a.persisted.revision.Revision,
			SpecHash:    result.SpecHash,
			Changes:     projectChangesFromProto(a.reconciled.changes),
		},
		ResourcesUnchanged: a.reconciled.agentsUnchanged,
	}, nil
}

func (a *projectApplyAdapter) result(ctx context.Context, foundation projectdomain.ApplyResult) (*projectApplyResult, error) {
	if a.schedulerFailure != nil {
		a.schedulerFailure.foundation = foundation
		return a.schedulerFailure, nil
	}
	if foundation.HasIssues() {
		return &projectApplyResult{
			foundation: foundation,
			issues:     a.issueProtos,
		}, nil
	}
	if foundation.DryRun {
		result := dryRunProjectApplyResult(a.normalized, a.resources)
		result.foundation = foundation
		return result, nil
	}
	return a.service.finalProjectApplyResult(ctx, a.normalized, a.persisted, a.resources, a.reconciled, foundation)
}
