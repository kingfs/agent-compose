package agentcompose

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	projectpkg "agent-compose/pkg/agentcompose/project"
	sqlitestore "agent-compose/pkg/agentcompose/store/sqlite"
	"agent-compose/pkg/compose"
)

const (
	ProjectRunStatusPending   = projectpkg.RunStatusPending
	ProjectRunStatusRunning   = projectpkg.RunStatusRunning
	ProjectRunStatusSucceeded = projectpkg.RunStatusSucceeded
	ProjectRunStatusFailed    = projectpkg.RunStatusFailed
	ProjectRunStatusCanceled  = projectpkg.RunStatusCanceled

	ProjectRunSourceManual    = projectpkg.RunSourceManual
	ProjectRunSourceScheduler = projectpkg.RunSourceScheduler
	ProjectRunSourceAPI       = projectpkg.RunSourceAPI
)

type ProjectRecord = projectpkg.ProjectRecord
type ProjectRevisionRecord = projectpkg.RevisionRecord
type ProjectAgentRecord = projectpkg.AgentRecord
type ProjectSchedulerRecord = projectpkg.SchedulerRecord
type ProjectRunRecord = projectpkg.RunRecord
type ProjectListOptions = projectpkg.ListOptions
type ProjectRunListOptions = projectpkg.RunListOptions
type ProjectListResult = projectpkg.ListResult

func StableProjectID(name, sourcePath string) (string, error) {
	return projectpkg.StableProjectID(name, sourcePath)
}

func StableManagedAgentID(projectID, agentName string) (string, error) {
	return projectpkg.StableManagedAgentID(projectID, agentName)
}

func StableProjectSchedulerID(projectID, agentName, schedulerName string) (string, error) {
	return projectpkg.StableSchedulerID(projectID, agentName, schedulerName)
}

func StableManagedLoaderID(projectID, agentName, schedulerName string) (string, error) {
	return projectpkg.StableManagedLoaderID(projectID, agentName, schedulerName)
}

func StableManagedTriggerID(projectID, agentName, schedulerName, triggerName string, triggerIndex int) (string, error) {
	return projectpkg.StableManagedTriggerID(projectID, agentName, schedulerName, triggerName, triggerIndex)
}

func StableProjectRunID(projectID, agentName, source, idempotencyKey string) (string, error) {
	return projectpkg.StableRunID(projectID, agentName, source, idempotencyKey)
}

func NewProjectRecordFromSpec(spec *compose.NormalizedProjectSpec, sourcePath string) (ProjectRecord, error) {
	return projectpkg.NewRecordFromSpec(spec, sourcePath)
}

func NewProjectAgentRecordFromSpec(projectID string, revision int64, agent compose.NormalizedAgentSpec) (ProjectAgentRecord, error) {
	return projectpkg.NewAgentRecordFromSpec(projectID, revision, agent)
}

func NewProjectSchedulerRecordFromSpec(projectID string, revision int64, agent compose.NormalizedAgentSpec) (ProjectSchedulerRecord, bool, error) {
	return projectpkg.NewSchedulerRecordFromSpec(projectID, revision, agent)
}

func (s *ConfigStore) projectRepository() *sqlitestore.ProjectRepository {
	return s.sqliteStore().ProjectRepository()
}

func (s *ConfigStore) UpsertProject(ctx context.Context, project ProjectRecord) (ProjectRecord, error) {
	return s.projectRepository().UpsertProject(ctx, project)
}

func (s *ConfigStore) SaveProjectRevision(ctx context.Context, revision ProjectRevisionRecord) (ProjectRevisionRecord, bool, error) {
	return s.projectRepository().SaveProjectRevision(ctx, revision)
}

func (s *ConfigStore) GetProject(ctx context.Context, projectID string) (ProjectRecord, error) {
	return s.projectRepository().GetProject(ctx, projectID)
}

func (s *ConfigStore) getProject(ctx context.Context, projectID string, includeRemoved bool) (ProjectRecord, bool, error) {
	return s.projectRepository().FindProject(ctx, projectID, includeRemoved)
}

func (s *ConfigStore) ListProjects(ctx context.Context, options ProjectListOptions) (ProjectListResult, error) {
	return s.projectRepository().ListProjects(ctx, options)
}

func (s *ConfigStore) GetProjectRevision(ctx context.Context, projectID string, revision int64) (ProjectRevisionRecord, error) {
	return s.projectRepository().GetProjectRevision(ctx, projectID, revision)
}

func (s *ConfigStore) UpsertProjectAgent(ctx context.Context, agent ProjectAgentRecord) (ProjectAgentRecord, error) {
	return s.projectRepository().UpsertProjectAgent(ctx, agent)
}

func (s *ConfigStore) GetProjectAgent(ctx context.Context, projectID, agentName string) (ProjectAgentRecord, error) {
	return s.projectRepository().GetProjectAgent(ctx, projectID, agentName)
}

func (s *ConfigStore) ListProjectAgents(ctx context.Context, projectID string) ([]ProjectAgentRecord, error) {
	return s.projectRepository().ListProjectAgents(ctx, projectID)
}

func (s *ConfigStore) UpsertProjectScheduler(ctx context.Context, scheduler ProjectSchedulerRecord) (ProjectSchedulerRecord, error) {
	return s.projectRepository().UpsertProjectScheduler(ctx, scheduler)
}

func (s *ConfigStore) GetProjectScheduler(ctx context.Context, projectID, schedulerID string) (ProjectSchedulerRecord, error) {
	return s.projectRepository().GetProjectScheduler(ctx, projectID, schedulerID)
}

func (s *ConfigStore) SetProjectSchedulerEnabled(ctx context.Context, projectID, schedulerID string, enabled bool) (ProjectSchedulerRecord, error) {
	return s.projectRepository().SetProjectSchedulerEnabled(ctx, projectID, schedulerID, enabled)
}

func (s *ConfigStore) ListProjectSchedulers(ctx context.Context, projectID string) ([]ProjectSchedulerRecord, error) {
	return s.projectRepository().ListProjectSchedulers(ctx, projectID)
}

func (s *ConfigStore) CreateProjectRun(ctx context.Context, run ProjectRunRecord) (ProjectRunRecord, error) {
	return s.projectRepository().CreateProjectRun(ctx, run)
}

func (s *ConfigStore) UpdateProjectRun(ctx context.Context, run ProjectRunRecord) (ProjectRunRecord, error) {
	return s.projectRepository().UpdateProjectRun(ctx, run)
}

func (s *ConfigStore) GetProjectRun(ctx context.Context, runID string) (ProjectRunRecord, error) {
	return s.projectRepository().GetProjectRun(ctx, runID)
}

func (s *ConfigStore) ListProjectRuns(ctx context.Context, projectID string, limit int) ([]ProjectRunRecord, error) {
	return s.projectRepository().ListProjectRuns(ctx, projectID, limit)
}

func (s *ConfigStore) ListProjectRunsByOptions(ctx context.Context, options ProjectRunListOptions) ([]ProjectRunRecord, error) {
	return s.projectRepository().ListProjectRunsByOptions(ctx, options)
}

func normalizeProjectRunStatus(status string) string {
	return projectpkg.NormalizeRunStatus(status)
}

func normalizeProjectSourcePath(sourcePath string) string {
	return projectpkg.NormalizeSourcePath(sourcePath)
}

func encodeProjectSourceJSON(sourcePath string) (string, error) {
	return projectpkg.EncodeSourceJSON(sourcePath)
}

func marshalCanonicalProjectJSON(value any) ([]byte, error) {
	return projectpkg.MarshalCanonicalJSON(value)
}

func stableReadableID(prefix, readable, seed string) string {
	readable = strings.ToLower(strings.TrimSpace(readable))
	var b strings.Builder
	for _, r := range readable {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	readable = strings.Trim(b.String(), "-_")
	if readable == "" {
		readable = "item"
	}
	if len(readable) > 48 {
		readable = strings.Trim(readable[:48], "-_")
	}
	sum := sha256.Sum256([]byte(seed))
	return prefix + "-" + readable + "-" + hex.EncodeToString(sum[:6])
}

func isProjectStableIdentifier(value string) bool {
	return projectpkg.IsStableIdentifier(value)
}
