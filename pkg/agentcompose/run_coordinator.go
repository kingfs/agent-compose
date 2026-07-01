package agentcompose

import (
	"context"
	"time"

	projectpkg "agent-compose/pkg/agentcompose/project"
	runpkg "agent-compose/pkg/agentcompose/run"
)

type ProjectRunStartRequest = runpkg.StartRequest
type ProjectRunTransitionRequest = runpkg.TransitionRequest

type RunCoordinator struct {
	inner *runpkg.Coordinator
	now   func() time.Time
}

func NewRunCoordinator(store *ConfigStore) *RunCoordinator {
	coordinator := &RunCoordinator{inner: runpkg.NewCoordinator(configStoreRunAdapter{store: store})}
	coordinator.now = func() time.Time { return time.Now().UTC() }
	return coordinator
}

func (c *RunCoordinator) BeginRun(ctx context.Context, req ProjectRunStartRequest) (ProjectRunRecord, error) {
	c.syncNow()
	record, err := c.inner.BeginRun(ctx, req)
	return ProjectRunRecord(record), err
}

func (c *RunCoordinator) MarkRunning(ctx context.Context, runID, sessionID string) (ProjectRunRecord, error) {
	c.syncNow()
	record, err := c.inner.MarkRunning(ctx, runID, sessionID)
	return ProjectRunRecord(record), err
}

func (c *RunCoordinator) MarkSucceeded(ctx context.Context, req ProjectRunTransitionRequest) (ProjectRunRecord, error) {
	c.syncNow()
	record, err := c.inner.MarkSucceeded(ctx, req)
	return ProjectRunRecord(record), err
}

func (c *RunCoordinator) MarkFailed(ctx context.Context, req ProjectRunTransitionRequest) (ProjectRunRecord, error) {
	c.syncNow()
	record, err := c.inner.MarkFailed(ctx, req)
	return ProjectRunRecord(record), err
}

func (c *RunCoordinator) MarkCanceled(ctx context.Context, req ProjectRunTransitionRequest) (ProjectRunRecord, error) {
	c.syncNow()
	record, err := c.inner.MarkCanceled(ctx, req)
	return ProjectRunRecord(record), err
}

func (c *RunCoordinator) TransitionRun(ctx context.Context, req ProjectRunTransitionRequest) (ProjectRunRecord, error) {
	c.syncNow()
	record, err := c.inner.TransitionRun(ctx, req)
	return ProjectRunRecord(record), err
}

func (c *RunCoordinator) syncNow() {
	if c != nil && c.inner != nil {
		c.inner.SetNow(c.now)
	}
}

func applyProjectRunTransitionFields(run *ProjectRunRecord, req ProjectRunTransitionRequest) {
	record := projectpkg.RunRecord(*run)
	runpkg.ApplyTransitionFields(&record, req)
	*run = ProjectRunRecord(record)
}

func validateProjectRunTransition(from, to string) error {
	return runpkg.ValidateTransition(from, to)
}

func projectRunStatusIsTerminal(status string) bool {
	return runpkg.StatusIsTerminal(status)
}

func normalizeProjectRunSource(source string) string {
	return projectpkg.NormalizeRunSource(source)
}

type configStoreRunAdapter struct {
	store *ConfigStore
}

func (a configStoreRunAdapter) GetProject(ctx context.Context, projectID string) (projectpkg.ProjectRecord, error) {
	record, err := a.store.GetProject(ctx, projectID)
	return projectpkg.ProjectRecord(record), err
}

func (a configStoreRunAdapter) GetProjectAgent(ctx context.Context, projectID, agentName string) (projectpkg.AgentRecord, error) {
	record, err := a.store.GetProjectAgent(ctx, projectID, agentName)
	return projectpkg.AgentRecord(record), err
}

func (a configStoreRunAdapter) GetManagedAgentDefinition(ctx context.Context, agentID string) (runpkg.ManagedAgentDefinition, error) {
	agent, err := a.store.GetAgentDefinition(ctx, agentID)
	if err != nil {
		return runpkg.ManagedAgentDefinition{}, err
	}
	return runpkg.ManagedAgentDefinition{
		ID:                     agent.ID,
		Enabled:                agent.Enabled,
		DeletedAt:              agent.DeletedAt,
		Driver:                 agent.Driver,
		GuestImage:             agent.GuestImage,
		ManagedProjectID:       agent.ManagedProjectID,
		ManagedProjectRevision: agent.ManagedProjectRevision,
		ManagedAgentName:       agent.ManagedAgentName,
	}, nil
}

func (a configStoreRunAdapter) CreateProjectRun(ctx context.Context, record projectpkg.RunRecord) (projectpkg.RunRecord, error) {
	created, err := a.store.CreateProjectRun(ctx, ProjectRunRecord(record))
	return projectpkg.RunRecord(created), err
}

func (a configStoreRunAdapter) UpdateProjectRun(ctx context.Context, record projectpkg.RunRecord) (projectpkg.RunRecord, error) {
	updated, err := a.store.UpdateProjectRun(ctx, ProjectRunRecord(record))
	return projectpkg.RunRecord(updated), err
}

func (a configStoreRunAdapter) GetProjectRun(ctx context.Context, runID string) (projectpkg.RunRecord, error) {
	record, err := a.store.GetProjectRun(ctx, runID)
	return projectpkg.RunRecord(record), err
}
