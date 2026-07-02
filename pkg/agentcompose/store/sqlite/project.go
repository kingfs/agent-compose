package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strconv"
	"strings"
	"time"

	"agent-compose/pkg/agentcompose/project"
)

const storedUnixMillisecondThreshold int64 = 10_000_000_000

type ProjectRepository struct {
	db *sql.DB
}

func (s *Store) ProjectRepository() *ProjectRepository {
	if s == nil {
		return nil
	}
	return &ProjectRepository{db: s.db}
}

func NewProjectRepository(db *sql.DB) *ProjectRepository {
	return &ProjectRepository{db: db}
}

func (r *ProjectRepository) UpsertProject(ctx context.Context, record project.ProjectRecord) (project.ProjectRecord, error) {
	record, err := project.NormalizeRecord(record)
	if err != nil {
		return project.ProjectRecord{}, err
	}
	now := time.Now().UTC()
	existing, found, err := r.getProject(ctx, record.ID, true)
	if err != nil {
		return project.ProjectRecord{}, err
	}
	if found {
		record.CreatedAt = existing.CreatedAt
		record.CurrentRevision = existing.CurrentRevision
		if record.SpecHash == "" {
			record.SpecHash = existing.SpecHash
		}
		record.UpdatedAt = now
		record.RemovedAt = time.Time{}
		result, err := r.db.ExecContext(ctx, `UPDATE project SET
			name = ?, source_path = ?, source_json = ?, spec_hash = ?, updated_at = ?, removed_at = 0
			WHERE id = ?`,
			record.Name, record.SourcePath, record.SourceJSON, record.SpecHash, record.UpdatedAt.Unix(), record.ID)
		if err != nil {
			return project.ProjectRecord{}, fmt.Errorf("update project %s: %w", record.ID, err)
		}
		if rows, _ := result.RowsAffected(); rows == 0 {
			return project.ProjectRecord{}, fmt.Errorf("project %s not found", record.ID)
		}
		return r.GetProject(ctx, record.ID)
	}

	record.CreatedAt = now
	record.UpdatedAt = now
	if _, err := r.db.ExecContext(ctx, `INSERT INTO project(
		id, name, source_path, source_json, current_revision, spec_hash, created_at, updated_at, removed_at
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, 0)`,
		record.ID, record.Name, record.SourcePath, record.SourceJSON, record.CurrentRevision, record.SpecHash, record.CreatedAt.Unix(), record.UpdatedAt.Unix()); err != nil {
		return project.ProjectRecord{}, fmt.Errorf("insert project %s: %w", record.ID, err)
	}
	return r.GetProject(ctx, record.ID)
}

func (r *ProjectRepository) SaveProjectRevision(ctx context.Context, revision project.RevisionRecord) (project.RevisionRecord, bool, error) {
	revision, err := project.NormalizeRevisionRecord(revision)
	if err != nil {
		return project.RevisionRecord{}, false, err
	}

	tx, err := r.db.BeginTx(ctx, nil)
	if err != nil {
		return project.RevisionRecord{}, false, fmt.Errorf("begin project revision tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	row := tx.QueryRowContext(ctx, `SELECT project_id, revision, spec_hash, spec_json, created_at
		FROM project_revision WHERE project_id = ? AND spec_hash = ?`, revision.ProjectID, revision.SpecHash)
	existing, err := scanProjectRevision(row.Scan)
	if err == nil {
		return existing, false, tx.Commit()
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return project.RevisionRecord{}, false, err
	}

	var nextRevision int64
	if err := tx.QueryRowContext(ctx, `SELECT COALESCE(MAX(revision), 0) + 1 FROM project_revision WHERE project_id = ?`, revision.ProjectID).Scan(&nextRevision); err != nil {
		return project.RevisionRecord{}, false, fmt.Errorf("query next project revision %s: %w", revision.ProjectID, err)
	}
	now := time.Now().UTC()
	revision.Revision = nextRevision
	revision.CreatedAt = now
	if _, err := tx.ExecContext(ctx, `INSERT INTO project_revision(project_id, revision, spec_hash, spec_json, created_at)
		VALUES(?, ?, ?, ?, ?)`, revision.ProjectID, revision.Revision, revision.SpecHash, revision.SpecJSON, revision.CreatedAt.Unix()); err != nil {
		return project.RevisionRecord{}, false, fmt.Errorf("insert project revision %s/%d: %w", revision.ProjectID, revision.Revision, err)
	}
	result, err := tx.ExecContext(ctx, `UPDATE project SET current_revision = ?, spec_hash = ?, updated_at = ?, removed_at = 0 WHERE id = ?`,
		revision.Revision, revision.SpecHash, now.Unix(), revision.ProjectID)
	if err != nil {
		return project.RevisionRecord{}, false, fmt.Errorf("update project revision pointer %s: %w", revision.ProjectID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return project.RevisionRecord{}, false, fmt.Errorf("project %s not found", revision.ProjectID)
	}
	if err := tx.Commit(); err != nil {
		return project.RevisionRecord{}, false, fmt.Errorf("commit project revision tx: %w", err)
	}
	return revision, true, nil
}

func (r *ProjectRepository) GetProject(ctx context.Context, projectID string) (project.ProjectRecord, error) {
	item, found, err := r.getProject(ctx, projectID, false)
	if err != nil {
		return project.ProjectRecord{}, err
	}
	if !found {
		return project.ProjectRecord{}, fmt.Errorf("project %s not found: %w", strings.TrimSpace(projectID), sql.ErrNoRows)
	}
	return item, nil
}

func (r *ProjectRepository) FindProject(ctx context.Context, projectID string, includeRemoved bool) (project.ProjectRecord, bool, error) {
	return r.getProject(ctx, projectID, includeRemoved)
}

func (r *ProjectRepository) ListProjects(ctx context.Context, options project.ListOptions) (project.ListResult, error) {
	limit := options.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := options.Offset
	if offset < 0 {
		offset = 0
	}
	rows, err := r.db.QueryContext(ctx, `SELECT id, name, source_path, source_json, current_revision, spec_hash, created_at, updated_at, removed_at
		FROM project ORDER BY updated_at DESC, created_at DESC, id ASC`)
	if err != nil {
		return project.ListResult{}, fmt.Errorf("query projects: %w", err)
	}
	defer func() { _ = rows.Close() }()

	query := strings.ToLower(strings.TrimSpace(options.Query))
	matched := make([]project.ProjectRecord, 0)
	for rows.Next() {
		item, err := scanProject(rows.Scan)
		if err != nil {
			return project.ListResult{}, err
		}
		if !options.IncludeRemoved && !item.RemovedAt.IsZero() {
			continue
		}
		if query != "" && !project.MatchesQuery(item, query) {
			continue
		}
		matched = append(matched, item)
	}
	if err := rows.Err(); err != nil {
		return project.ListResult{}, fmt.Errorf("iterate projects: %w", err)
	}
	total := len(matched)
	end := offset + limit
	if offset > total {
		offset = total
	}
	if end > total {
		end = total
	}
	return project.ListResult{
		Projects:   matched[offset:end],
		TotalCount: total,
		HasMore:    end < total,
		NextOffset: end,
	}, nil
}

func (r *ProjectRepository) GetProjectRevision(ctx context.Context, projectID string, revision int64) (project.RevisionRecord, error) {
	row := r.db.QueryRowContext(ctx, `SELECT project_id, revision, spec_hash, spec_json, created_at
		FROM project_revision WHERE project_id = ? AND revision = ?`, strings.TrimSpace(projectID), revision)
	item, err := scanProjectRevision(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return project.RevisionRecord{}, fmt.Errorf("project revision %s/%d not found: %w", strings.TrimSpace(projectID), revision, err)
		}
		return project.RevisionRecord{}, err
	}
	return item, nil
}

func (r *ProjectRepository) UpsertProjectAgent(ctx context.Context, agent project.AgentRecord) (project.AgentRecord, error) {
	agent, err := project.NormalizeAgentRecord(agent)
	if err != nil {
		return project.AgentRecord{}, err
	}
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `UPDATE project_agent SET
		managed_agent_id = ?, revision = ?, provider = ?, model = ?, image = ?, driver = ?, scheduler_enabled = ?, spec_json = ?, updated_at = ?
		WHERE project_id = ? AND agent_name = ?`,
		agent.ManagedAgentID, agent.Revision, agent.Provider, agent.Model, agent.Image, agent.Driver, boolToInt(agent.SchedulerEnabled), agent.SpecJSON, now.Unix(),
		agent.ProjectID, agent.AgentName)
	if err != nil {
		return project.AgentRecord{}, fmt.Errorf("update project agent %s/%s: %w", agent.ProjectID, agent.AgentName, err)
	}
	if rows, _ := result.RowsAffected(); rows > 0 {
		return r.GetProjectAgent(ctx, agent.ProjectID, agent.AgentName)
	}
	agent.CreatedAt = now
	agent.UpdatedAt = now
	if _, err := r.db.ExecContext(ctx, `INSERT INTO project_agent(
		project_id, agent_name, managed_agent_id, revision, provider, model, image, driver, scheduler_enabled, spec_json, created_at, updated_at
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		agent.ProjectID, agent.AgentName, agent.ManagedAgentID, agent.Revision, agent.Provider, agent.Model, agent.Image, agent.Driver, boolToInt(agent.SchedulerEnabled), agent.SpecJSON,
		agent.CreatedAt.Unix(), agent.UpdatedAt.Unix()); err != nil {
		return project.AgentRecord{}, fmt.Errorf("insert project agent %s/%s: %w", agent.ProjectID, agent.AgentName, err)
	}
	return r.GetProjectAgent(ctx, agent.ProjectID, agent.AgentName)
}

func (r *ProjectRepository) GetProjectAgent(ctx context.Context, projectID, agentName string) (project.AgentRecord, error) {
	row := r.db.QueryRowContext(ctx, `SELECT project_id, agent_name, managed_agent_id, revision, provider, model, image, driver, scheduler_enabled, spec_json, created_at, updated_at
		FROM project_agent WHERE project_id = ? AND agent_name = ?`, strings.TrimSpace(projectID), strings.TrimSpace(agentName))
	item, err := scanProjectAgent(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return project.AgentRecord{}, fmt.Errorf("project agent %s/%s not found: %w", strings.TrimSpace(projectID), strings.TrimSpace(agentName), err)
		}
		return project.AgentRecord{}, err
	}
	return item, nil
}

func (r *ProjectRepository) ListProjectAgents(ctx context.Context, projectID string) ([]project.AgentRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT project_id, agent_name, managed_agent_id, revision, provider, model, image, driver, scheduler_enabled, spec_json, created_at, updated_at
		FROM project_agent WHERE project_id = ? ORDER BY agent_name ASC`, strings.TrimSpace(projectID))
	if err != nil {
		return nil, fmt.Errorf("query project agents %s: %w", strings.TrimSpace(projectID), err)
	}
	defer func() { _ = rows.Close() }()
	var items []project.AgentRecord
	for rows.Next() {
		item, err := scanProjectAgent(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project agents %s: %w", strings.TrimSpace(projectID), err)
	}
	return items, nil
}

func (r *ProjectRepository) UpsertProjectScheduler(ctx context.Context, scheduler project.SchedulerRecord) (project.SchedulerRecord, error) {
	scheduler, err := project.NormalizeSchedulerRecord(scheduler)
	if err != nil {
		return project.SchedulerRecord{}, err
	}
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `UPDATE project_scheduler SET
		agent_name = ?, managed_loader_id = ?, revision = ?, enabled = ?, trigger_count = ?, spec_json = ?, updated_at = ?
		WHERE project_id = ? AND scheduler_id = ?`,
		scheduler.AgentName, scheduler.ManagedLoaderID, scheduler.Revision, boolToInt(scheduler.Enabled), scheduler.TriggerCount, scheduler.SpecJSON, now.Unix(),
		scheduler.ProjectID, scheduler.SchedulerID)
	if err != nil {
		return project.SchedulerRecord{}, fmt.Errorf("update project scheduler %s/%s: %w", scheduler.ProjectID, scheduler.SchedulerID, err)
	}
	if rows, _ := result.RowsAffected(); rows > 0 {
		return r.GetProjectScheduler(ctx, scheduler.ProjectID, scheduler.SchedulerID)
	}
	scheduler.CreatedAt = now
	scheduler.UpdatedAt = now
	if _, err := r.db.ExecContext(ctx, `INSERT INTO project_scheduler(
		project_id, scheduler_id, agent_name, managed_loader_id, revision, enabled, trigger_count, spec_json, created_at, updated_at
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		scheduler.ProjectID, scheduler.SchedulerID, scheduler.AgentName, scheduler.ManagedLoaderID, scheduler.Revision, boolToInt(scheduler.Enabled), scheduler.TriggerCount, scheduler.SpecJSON,
		scheduler.CreatedAt.Unix(), scheduler.UpdatedAt.Unix()); err != nil {
		return project.SchedulerRecord{}, fmt.Errorf("insert project scheduler %s/%s: %w", scheduler.ProjectID, scheduler.SchedulerID, err)
	}
	return r.GetProjectScheduler(ctx, scheduler.ProjectID, scheduler.SchedulerID)
}

func (r *ProjectRepository) GetProjectScheduler(ctx context.Context, projectID, schedulerID string) (project.SchedulerRecord, error) {
	row := r.db.QueryRowContext(ctx, `SELECT project_id, scheduler_id, agent_name, managed_loader_id, revision, enabled, trigger_count, spec_json, created_at, updated_at
		FROM project_scheduler WHERE project_id = ? AND scheduler_id = ?`, strings.TrimSpace(projectID), strings.TrimSpace(schedulerID))
	item, err := scanProjectScheduler(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return project.SchedulerRecord{}, fmt.Errorf("project scheduler %s/%s not found: %w", strings.TrimSpace(projectID), strings.TrimSpace(schedulerID), err)
		}
		return project.SchedulerRecord{}, err
	}
	return item, nil
}

func (r *ProjectRepository) SetProjectSchedulerEnabled(ctx context.Context, projectID, schedulerID string, enabled bool) (project.SchedulerRecord, error) {
	projectID = strings.TrimSpace(projectID)
	schedulerID = strings.TrimSpace(schedulerID)
	if projectID == "" || schedulerID == "" {
		return project.SchedulerRecord{}, fmt.Errorf("project scheduler id is required")
	}
	result, err := r.db.ExecContext(ctx, `UPDATE project_scheduler SET enabled = ?, updated_at = ? WHERE project_id = ? AND scheduler_id = ?`,
		boolToInt(enabled), time.Now().UTC().Unix(), projectID, schedulerID)
	if err != nil {
		return project.SchedulerRecord{}, fmt.Errorf("update project scheduler %s/%s enabled state: %w", projectID, schedulerID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return project.SchedulerRecord{}, fmt.Errorf("project scheduler %s/%s not found", projectID, schedulerID)
	}
	return r.GetProjectScheduler(ctx, projectID, schedulerID)
}

func (r *ProjectRepository) ListProjectSchedulers(ctx context.Context, projectID string) ([]project.SchedulerRecord, error) {
	rows, err := r.db.QueryContext(ctx, `SELECT project_id, scheduler_id, agent_name, managed_loader_id, revision, enabled, trigger_count, spec_json, created_at, updated_at
		FROM project_scheduler WHERE project_id = ? ORDER BY agent_name ASC, scheduler_id ASC`, strings.TrimSpace(projectID))
	if err != nil {
		return nil, fmt.Errorf("query project schedulers %s: %w", strings.TrimSpace(projectID), err)
	}
	defer func() { _ = rows.Close() }()
	var items []project.SchedulerRecord
	for rows.Next() {
		item, err := scanProjectScheduler(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project schedulers %s: %w", strings.TrimSpace(projectID), err)
	}
	return items, nil
}

func (r *ProjectRepository) CreateProjectRun(ctx context.Context, run project.RunRecord) (project.RunRecord, error) {
	run, err := project.NormalizeRunRecord(run)
	if err != nil {
		return project.RunRecord{}, err
	}
	now := time.Now().UTC()
	run.CreatedAt = now
	run.UpdatedAt = now
	if _, err := r.db.ExecContext(ctx, `INSERT INTO project_run(
		run_id, project_id, project_name, project_revision, agent_name, managed_agent_id, source, scheduler_id, trigger_id, status,
		session_id, exit_code, error, prompt, output, result_json, logs_path, artifacts_dir, cleanup_error, driver, image_ref,
		started_at, completed_at, duration_ms, created_at, updated_at
	) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		run.RunID, run.ProjectID, run.ProjectName, run.ProjectRevision, run.AgentName, run.ManagedAgentID, run.Source, run.SchedulerID, run.TriggerID, run.Status,
		run.SessionID, run.ExitCode, run.Error, run.Prompt, run.Output, run.ResultJSON, run.LogsPath, run.ArtifactsDir, run.CleanupError, run.Driver, run.ImageRef,
		nonZeroTimeUnixMilli(run.StartedAt), nonZeroTimeUnixMilli(run.CompletedAt), run.DurationMs, run.CreatedAt.Unix(), run.UpdatedAt.Unix()); err != nil {
		return project.RunRecord{}, fmt.Errorf("insert project run %s: %w", run.RunID, err)
	}
	return r.GetProjectRun(ctx, run.RunID)
}

func (r *ProjectRepository) UpdateProjectRun(ctx context.Context, run project.RunRecord) (project.RunRecord, error) {
	run, err := project.NormalizeRunRecord(run)
	if err != nil {
		return project.RunRecord{}, err
	}
	now := time.Now().UTC()
	result, err := r.db.ExecContext(ctx, `UPDATE project_run SET
		project_id = ?, project_name = ?, project_revision = ?, agent_name = ?, managed_agent_id = ?, source = ?, scheduler_id = ?, trigger_id = ?, status = ?,
		session_id = ?, exit_code = ?, error = ?, prompt = ?, output = ?, result_json = ?, logs_path = ?, artifacts_dir = ?, cleanup_error = ?, driver = ?, image_ref = ?,
		started_at = ?, completed_at = ?, duration_ms = ?, updated_at = ?
		WHERE run_id = ?`,
		run.ProjectID, run.ProjectName, run.ProjectRevision, run.AgentName, run.ManagedAgentID, run.Source, run.SchedulerID, run.TriggerID, run.Status,
		run.SessionID, run.ExitCode, run.Error, run.Prompt, run.Output, run.ResultJSON, run.LogsPath, run.ArtifactsDir, run.CleanupError, run.Driver, run.ImageRef,
		nonZeroTimeUnixMilli(run.StartedAt), nonZeroTimeUnixMilli(run.CompletedAt), run.DurationMs, now.Unix(), run.RunID)
	if err != nil {
		return project.RunRecord{}, fmt.Errorf("update project run %s: %w", run.RunID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return project.RunRecord{}, fmt.Errorf("project run %s not found", run.RunID)
	}
	return r.GetProjectRun(ctx, run.RunID)
}

func (r *ProjectRepository) GetProjectRun(ctx context.Context, runID string) (project.RunRecord, error) {
	row := r.db.QueryRowContext(ctx, selectProjectRunSQL()+` WHERE run_id = ?`, strings.TrimSpace(runID))
	item, err := scanProjectRun(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return project.RunRecord{}, fmt.Errorf("project run %s not found: %w", strings.TrimSpace(runID), err)
		}
		return project.RunRecord{}, err
	}
	return item, nil
}

func (r *ProjectRepository) ListProjectRuns(ctx context.Context, projectID string, limit int) ([]project.RunRecord, error) {
	return r.ListProjectRunsByOptions(ctx, project.RunListOptions{ProjectID: projectID, Limit: limit})
}

func (r *ProjectRepository) ListProjectRunsByOptions(ctx context.Context, options project.RunListOptions) ([]project.RunRecord, error) {
	limit := options.Limit
	if limit <= 0 {
		limit = 50
	}
	if limit > 200 {
		limit = 200
	}
	offset := options.Offset
	if offset < 0 {
		offset = 0
	}
	where := make([]string, 0, 6)
	args := make([]any, 0, 8)
	if projectID := strings.TrimSpace(options.ProjectID); projectID != "" {
		where = append(where, "project_id = ?")
		args = append(args, projectID)
	}
	if agentName := strings.TrimSpace(options.AgentName); agentName != "" {
		where = append(where, "agent_name = ?")
		args = append(args, agentName)
	}
	if sessionID := strings.TrimSpace(options.SessionID); sessionID != "" {
		where = append(where, "session_id = ?")
		args = append(args, sessionID)
	}
	if schedulerID := strings.TrimSpace(options.SchedulerID); schedulerID != "" {
		where = append(where, "scheduler_id = ?")
		args = append(args, schedulerID)
	}
	if status := strings.TrimSpace(options.Status); status != "" {
		where = append(where, "status = ?")
		args = append(args, project.NormalizeRunStatus(status))
	}
	if source := strings.TrimSpace(options.Source); source != "" {
		where = append(where, "source = ?")
		args = append(args, project.NormalizeRunSource(source))
	}
	query := selectProjectRunSQL()
	if len(where) > 0 {
		query += ` WHERE ` + strings.Join(where, ` AND `)
	}
	query += ` ORDER BY created_at DESC, run_id DESC LIMIT ? OFFSET ?`
	args = append(args, limit, offset)
	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query project runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var items []project.RunRecord
	for rows.Next() {
		item, err := scanProjectRun(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project runs: %w", err)
	}
	return items, nil
}

func (r *ProjectRepository) ListProjectSessionRuns(ctx context.Context, filter project.SessionRelationFilter) ([]project.RunRecord, error) {
	query := selectProjectRunSQL() + ` WHERE session_id != ''`
	args := make([]any, 0, 4+len(filter.Statuses))
	if projectID := strings.TrimSpace(filter.ProjectID); projectID != "" {
		query += ` AND project_id = ?`
		args = append(args, projectID)
	}
	if agentName := strings.TrimSpace(filter.AgentName); agentName != "" {
		query += ` AND agent_name = ?`
		args = append(args, agentName)
	}
	if sessionID := strings.TrimSpace(filter.SessionID); sessionID != "" {
		query += ` AND session_id = ?`
		args = append(args, sessionID)
	}
	statuses := project.NormalizeRunStatusFilter(filter.Statuses)
	if len(statuses) > 0 {
		query += ` AND status IN (` + project.Placeholders(len(statuses)) + `)`
		for _, status := range statuses {
			args = append(args, status)
		}
	}
	query += ` ORDER BY updated_at DESC, created_at DESC, run_id DESC`
	limit := filter.Limit
	if limit <= 0 {
		limit = 200
	}
	if limit > 500 {
		limit = 500
	}
	query += ` LIMIT ?`
	args = append(args, limit)

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("query project session runs: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var items []project.RunRecord
	for rows.Next() {
		item, err := scanProjectRun(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate project session runs: %w", err)
	}
	return items, nil
}

func (r *ProjectRepository) ListProjectRunsForSession(ctx context.Context, sessionID string) ([]project.RunRecord, error) {
	sessionID = strings.TrimSpace(sessionID)
	if sessionID == "" {
		return nil, fmt.Errorf("session id is required")
	}
	return r.ListProjectSessionRuns(ctx, project.SessionRelationFilter{SessionID: sessionID})
}

func (r *ProjectRepository) getProject(ctx context.Context, projectID string, includeRemoved bool) (project.ProjectRecord, bool, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return project.ProjectRecord{}, false, fmt.Errorf("project id is required")
	}
	where := "id = ? AND removed_at = 0"
	if includeRemoved {
		where = "id = ?"
	}
	row := r.db.QueryRowContext(ctx, `SELECT id, name, source_path, source_json, current_revision, spec_hash, created_at, updated_at, removed_at
		FROM project WHERE `+where, projectID)
	item, err := scanProject(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return project.ProjectRecord{}, false, nil
		}
		return project.ProjectRecord{}, false, err
	}
	return item, true, nil
}

func scanProject(scan func(dest ...any) error) (project.ProjectRecord, error) {
	var item project.ProjectRecord
	var createdAtRaw any
	var updatedAtRaw any
	var removedAtRaw any
	if err := scan(&item.ID, &item.Name, &item.SourcePath, &item.SourceJSON, &item.CurrentRevision, &item.SpecHash, &createdAtRaw, &updatedAtRaw, &removedAtRaw); err != nil {
		return project.ProjectRecord{}, fmt.Errorf("scan project: %w", err)
	}
	item.CreatedAt = parseStoredTime(createdAtRaw)
	item.UpdatedAt = parseStoredTime(updatedAtRaw)
	item.RemovedAt = parseStoredTime(removedAtRaw)
	return item, nil
}

func scanProjectRevision(scan func(dest ...any) error) (project.RevisionRecord, error) {
	var item project.RevisionRecord
	var createdAtRaw any
	if err := scan(&item.ProjectID, &item.Revision, &item.SpecHash, &item.SpecJSON, &createdAtRaw); err != nil {
		return project.RevisionRecord{}, fmt.Errorf("scan project revision: %w", err)
	}
	item.CreatedAt = parseStoredTime(createdAtRaw)
	return item, nil
}

func scanProjectAgent(scan func(dest ...any) error) (project.AgentRecord, error) {
	var item project.AgentRecord
	var schedulerEnabled int
	var createdAtRaw any
	var updatedAtRaw any
	if err := scan(&item.ProjectID, &item.AgentName, &item.ManagedAgentID, &item.Revision, &item.Provider, &item.Model, &item.Image, &item.Driver, &schedulerEnabled, &item.SpecJSON, &createdAtRaw, &updatedAtRaw); err != nil {
		return project.AgentRecord{}, fmt.Errorf("scan project agent: %w", err)
	}
	item.SchedulerEnabled = schedulerEnabled != 0
	item.CreatedAt = parseStoredTime(createdAtRaw)
	item.UpdatedAt = parseStoredTime(updatedAtRaw)
	return item, nil
}

func scanProjectScheduler(scan func(dest ...any) error) (project.SchedulerRecord, error) {
	var item project.SchedulerRecord
	var enabled int
	var createdAtRaw any
	var updatedAtRaw any
	if err := scan(&item.ProjectID, &item.SchedulerID, &item.AgentName, &item.ManagedLoaderID, &item.Revision, &enabled, &item.TriggerCount, &item.SpecJSON, &createdAtRaw, &updatedAtRaw); err != nil {
		return project.SchedulerRecord{}, fmt.Errorf("scan project scheduler: %w", err)
	}
	item.Enabled = enabled != 0
	item.CreatedAt = parseStoredTime(createdAtRaw)
	item.UpdatedAt = parseStoredTime(updatedAtRaw)
	return item, nil
}

func scanProjectRun(scan func(dest ...any) error) (project.RunRecord, error) {
	var item project.RunRecord
	var startedAtRaw any
	var completedAtRaw any
	var createdAtRaw any
	var updatedAtRaw any
	if err := scan(
		&item.RunID, &item.ProjectID, &item.ProjectName, &item.ProjectRevision, &item.AgentName, &item.ManagedAgentID, &item.Source, &item.SchedulerID, &item.TriggerID, &item.Status,
		&item.SessionID, &item.ExitCode, &item.Error, &item.Prompt, &item.Output, &item.ResultJSON, &item.LogsPath, &item.ArtifactsDir, &item.CleanupError, &item.Driver, &item.ImageRef,
		&startedAtRaw, &completedAtRaw, &item.DurationMs, &createdAtRaw, &updatedAtRaw,
	); err != nil {
		return project.RunRecord{}, fmt.Errorf("scan project run: %w", err)
	}
	item.StartedAt = parseStoredUnixTimeAuto(asInt64Time(startedAtRaw))
	item.CompletedAt = parseStoredUnixTimeAuto(asInt64Time(completedAtRaw))
	item.CreatedAt = parseStoredTime(createdAtRaw)
	item.UpdatedAt = parseStoredTime(updatedAtRaw)
	return item, nil
}

func selectProjectRunSQL() string {
	return `SELECT run_id, project_id, project_name, project_revision, agent_name, managed_agent_id, source, scheduler_id, trigger_id, status,
		session_id, exit_code, error, prompt, output, result_json, logs_path, artifacts_dir, cleanup_error, driver, image_ref,
		started_at, completed_at, duration_ms, created_at, updated_at FROM project_run`
}

func nonZeroTimeUnixMilli(value time.Time) int64 {
	if value.IsZero() {
		return 0
	}
	return value.UTC().UnixMilli()
}

func parseStoredUnixTimeAuto(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	if value >= storedUnixMillisecondThreshold {
		return time.UnixMilli(value).UTC()
	}
	return time.Unix(value, 0).UTC()
}

func parseStoredTime(value any) time.Time {
	switch typed := value.(type) {
	case nil:
		return time.Time{}
	case int64:
		return parseStoredUnixTimeAuto(typed)
	case int:
		return parseStoredUnixTimeAuto(int64(typed))
	case float64:
		return parseStoredUnixTimeAuto(int64(typed))
	case []byte:
		return parseStoredTime(string(typed))
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return time.Time{}
		}
		if unixValue, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return parseStoredUnixTimeAuto(unixValue)
		}
		for _, layout := range []string{time.RFC3339Nano, time.RFC3339, "2006-01-02T15:04:05.000Z"} {
			if parsed, err := time.Parse(layout, trimmed); err == nil {
				return parsed.UTC()
			}
		}
	}
	return time.Time{}
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}

func asInt64Time(value any) int64 {
	switch typed := value.(type) {
	case nil:
		return 0
	case int64:
		return typed
	case int:
		return int64(typed)
	case float64:
		return int64(typed)
	case []byte:
		return asInt64Time(string(typed))
	case string:
		parsed, _ := parseInt64String(typed)
		return parsed
	default:
		return 0
	}
}

func parseInt64String(value string) (int64, bool) {
	value = strings.TrimSpace(value)
	if value == "" {
		return 0, false
	}
	var parsed int64
	for _, r := range value {
		if r < '0' || r > '9' {
			return 0, false
		}
		parsed = parsed*10 + int64(r-'0')
	}
	return parsed, true
}
