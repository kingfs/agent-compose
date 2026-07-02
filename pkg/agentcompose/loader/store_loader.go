package loader

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) CreateLoader(ctx context.Context, item Loader) (Loader, error) {
	normalized, err := normalizeLoader(item, true)
	if err != nil {
		return Loader{}, err
	}
	envJSON, err := encodeLoaderEnvItems(normalized.EnvItems)
	if err != nil {
		return Loader{}, err
	}
	capsetIDsJSON, err := encodeCapsetIDs(normalized.Summary.CapsetIDs)
	if err != nil {
		return Loader{}, err
	}
	now := time.Now().UTC()
	normalized.Summary.CreatedAt = now
	normalized.Summary.UpdatedAt = now
	_, err = s.db.ExecContext(ctx, `INSERT INTO loader(
        id, name, description, runtime, script, workspace_id, agent_id, driver, guest_image, default_agent, session_policy, concurrency_policy, capset_ids, env_json,
        managed_project_id, managed_project_revision, managed_agent_name, managed_scheduler_id, enabled, last_error, created_at, updated_at
    ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		normalized.Summary.ID,
		normalized.Summary.Name,
		normalized.Summary.Description,
		normalized.Summary.Runtime,
		normalized.Script,
		normalized.Summary.WorkspaceID,
		normalized.Summary.AgentID,
		normalized.Summary.Driver,
		normalized.Summary.GuestImage,
		normalized.Summary.DefaultAgent,
		normalized.Summary.SessionPolicy,
		normalized.Summary.ConcurrencyPolicy,
		capsetIDsJSON,
		envJSON,
		normalized.Summary.ManagedProjectID,
		normalized.Summary.ManagedRevision,
		normalized.Summary.ManagedAgentName,
		normalized.Summary.ManagedSchedulerID,
		boolToInt(normalized.Summary.Enabled),
		normalized.Summary.LastError,
		normalized.Summary.CreatedAt.Unix(),
		normalized.Summary.UpdatedAt.Unix(),
	)
	if err != nil {
		return Loader{}, fmt.Errorf("insert loader %s: %w", normalized.Summary.ID, err)
	}
	return normalized, nil
}

func (s *Store) UpdateLoader(ctx context.Context, item Loader) (Loader, error) {
	normalized, err := normalizeLoader(item, false)
	if err != nil {
		return Loader{}, err
	}
	existing, err := s.GetLoader(ctx, normalized.Summary.ID)
	if err != nil {
		return Loader{}, err
	}
	if normalized.Summary.ManagedProjectID == "" && normalized.Summary.ManagedAgentName == "" && normalized.Summary.ManagedSchedulerID == "" && normalized.Summary.ManagedRevision == 0 {
		normalized.Summary.ManagedProjectID = existing.Summary.ManagedProjectID
		normalized.Summary.ManagedRevision = existing.Summary.ManagedRevision
		normalized.Summary.ManagedAgentName = existing.Summary.ManagedAgentName
		normalized.Summary.ManagedSchedulerID = existing.Summary.ManagedSchedulerID
	}
	envJSON, err := encodeLoaderEnvItems(normalized.EnvItems)
	if err != nil {
		return Loader{}, err
	}
	capsetIDsJSON, err := encodeCapsetIDs(normalized.Summary.CapsetIDs)
	if err != nil {
		return Loader{}, err
	}
	normalized.Summary.CreatedAt = existing.Summary.CreatedAt
	normalized.Summary.UpdatedAt = time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE loader SET
        name = ?, description = ?, runtime = ?, script = ?, workspace_id = ?, agent_id = ?, driver = ?, guest_image = ?, default_agent = ?, session_policy = ?,
        concurrency_policy = ?, capset_ids = ?, env_json = ?, managed_project_id = ?, managed_project_revision = ?, managed_agent_name = ?, managed_scheduler_id = ?,
        enabled = ?, last_error = ?, updated_at = ?
        WHERE id = ?`,
		normalized.Summary.Name,
		normalized.Summary.Description,
		normalized.Summary.Runtime,
		normalized.Script,
		normalized.Summary.WorkspaceID,
		normalized.Summary.AgentID,
		normalized.Summary.Driver,
		normalized.Summary.GuestImage,
		normalized.Summary.DefaultAgent,
		normalized.Summary.SessionPolicy,
		normalized.Summary.ConcurrencyPolicy,
		capsetIDsJSON,
		envJSON,
		normalized.Summary.ManagedProjectID,
		normalized.Summary.ManagedRevision,
		normalized.Summary.ManagedAgentName,
		normalized.Summary.ManagedSchedulerID,
		boolToInt(normalized.Summary.Enabled),
		normalized.Summary.LastError,
		normalized.Summary.UpdatedAt.Unix(),
		normalized.Summary.ID,
	)
	if err != nil {
		return Loader{}, fmt.Errorf("update loader %s: %w", normalized.Summary.ID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return Loader{}, fmt.Errorf("loader %s not found", normalized.Summary.ID)
	}
	normalized.Summary.TriggerCount = existing.Summary.TriggerCount
	normalized.Summary.RunCount = existing.Summary.RunCount
	normalized.Summary.EventCount = existing.Summary.EventCount
	normalized.Summary.LatestRunAt = existing.Summary.LatestRunAt
	return normalized, nil
}

func (s *Store) UpsertManagedLoader(ctx context.Context, item Loader) (Loader, error) {
	normalized, err := normalizeLoader(item, false)
	if err != nil {
		return Loader{}, err
	}
	if normalized.Summary.ManagedProjectID == "" || normalized.Summary.ManagedAgentName == "" || normalized.Summary.ManagedSchedulerID == "" {
		return Loader{}, fmt.Errorf("managed project id, agent name, and scheduler id are required")
	}
	if existing, found, err := s.getLoaderIfExists(ctx, normalized.Summary.ID); err != nil {
		return Loader{}, err
	} else if found {
		normalized.Summary.CreatedAt = existing.Summary.CreatedAt
		return s.UpdateLoader(ctx, normalized)
	}
	return s.CreateLoader(ctx, normalized)
}

func (s *Store) getLoaderIfExists(ctx context.Context, loaderID string) (Loader, bool, error) {
	item, err := s.GetLoader(ctx, loaderID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Loader{}, false, nil
		}
		return Loader{}, false, err
	}
	return item, true, nil
}

func (s *Store) DeleteLoader(ctx context.Context, loaderID string) error {
	loaderID = strings.TrimSpace(loaderID)
	if loaderID == "" {
		return fmt.Errorf("loader id is required")
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM loader WHERE id = ?`, loaderID)
	if err != nil {
		return fmt.Errorf("delete loader %s: %w", loaderID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("loader %s not found", loaderID)
	}
	_, _ = s.db.ExecContext(ctx, `DELETE FROM loader_binding WHERE loader_id = ?`, loaderID)
	return nil
}

func (s *Store) DisableLoadersByDefaultAgent(ctx context.Context, agentID string) (int, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return 0, fmt.Errorf("agent id is required")
	}
	now := time.Now().UTC().Unix()
	result, err := s.db.ExecContext(ctx, `UPDATE loader SET enabled = 0, updated_at = ? WHERE (agent_id = ? OR default_agent = ?) AND enabled = 1`, now, agentID, agentID)
	if err != nil {
		return 0, fmt.Errorf("disable loaders for agent %s: %w", agentID, err)
	}
	rows, _ := result.RowsAffected()
	return int(rows), nil
}

func (s *Store) ListLoaderSummaries(ctx context.Context) ([]LoaderSummary, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT
        l.id,
        l.name,
        l.description,
        l.runtime,
        l.workspace_id,
        l.agent_id,
        l.driver,
        l.guest_image,
        l.default_agent,
        l.session_policy,
        l.concurrency_policy,
        l.capset_ids,
        l.managed_project_id,
        l.managed_project_revision,
        l.managed_agent_name,
        l.managed_scheduler_id,
        l.enabled,
        l.last_error,
        l.created_at,
        l.updated_at,
        (SELECT COUNT(*) FROM loader_trigger t WHERE t.loader_id = l.id),
        (SELECT COUNT(*) FROM loader_run r WHERE r.loader_id = l.id),
        (SELECT COUNT(*) FROM loader_event e WHERE e.loader_id = l.id),
        (SELECT MAX(r.started_at) FROM loader_run r WHERE r.loader_id = l.id)
        FROM loader l
        ORDER BY l.updated_at DESC, l.created_at DESC, l.id DESC`)
	if err != nil {
		return nil, fmt.Errorf("query loaders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]LoaderSummary, 0)
	for rows.Next() {
		item, err := scanLoaderSummary(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate loaders: %w", err)
	}
	return items, nil
}

func (s *Store) GetLoader(ctx context.Context, loaderID string) (Loader, error) {
	loaderID = strings.TrimSpace(loaderID)
	if loaderID == "" {
		return Loader{}, fmt.Errorf("loader id is required")
	}
	row := s.db.QueryRowContext(ctx, `SELECT
        id, name, description, runtime, script, workspace_id, agent_id, driver, guest_image, default_agent, session_policy, concurrency_policy, capset_ids, env_json,
        managed_project_id, managed_project_revision, managed_agent_name, managed_scheduler_id, enabled, last_error, created_at, updated_at
        FROM loader WHERE id = ?`, loaderID)
	item, err := scanLoader(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return Loader{}, fmt.Errorf("loader %s not found: %w", loaderID, err)
		}
		return Loader{}, err
	}
	if err := s.hydrateLoaderSummaryCounts(ctx, &item.Summary); err != nil {
		return Loader{}, err
	}
	triggers, err := s.listLoaderTriggers(ctx, loaderID)
	if err != nil {
		return Loader{}, err
	}
	item.Triggers = triggers
	return item, nil
}

func (s *Store) ListLoaders(ctx context.Context) ([]Loader, error) {
	summaries, err := s.ListLoaderSummaries(ctx)
	if err != nil {
		return nil, err
	}
	items := make([]Loader, 0, len(summaries))
	for _, summary := range summaries {
		item, err := s.GetLoader(ctx, summary.ID)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Store) ListManagedLoaders(ctx context.Context, projectID string) ([]Loader, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("project id is required")
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id FROM loader WHERE managed_project_id = ? ORDER BY managed_agent_name ASC, managed_scheduler_id ASC, id ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query managed loaders %s: %w", projectID, err)
	}
	defer func() { _ = rows.Close() }()
	var ids []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan managed loader id: %w", err)
		}
		ids = append(ids, id)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate managed loaders %s: %w", projectID, err)
	}
	items := make([]Loader, 0, len(ids))
	for _, id := range ids {
		item, err := s.GetLoader(ctx, id)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	return items, nil
}

func (s *Store) hydrateLoaderSummaryCounts(ctx context.Context, summary *LoaderSummary) error {
	if summary == nil || strings.TrimSpace(summary.ID) == "" {
		return nil
	}
	row := s.db.QueryRowContext(ctx, `SELECT
        (SELECT COUNT(*) FROM loader_trigger WHERE loader_id = ?),
        (SELECT COUNT(*) FROM loader_run WHERE loader_id = ?),
        (SELECT COUNT(*) FROM loader_event WHERE loader_id = ?),
        (SELECT MAX(started_at) FROM loader_run WHERE loader_id = ?)`, summary.ID, summary.ID, summary.ID, summary.ID)
	var triggerCount int
	var runCount int
	var eventCount int
	var latestRunAtRaw any
	if err := row.Scan(&triggerCount, &runCount, &eventCount, &latestRunAtRaw); err != nil {
		return fmt.Errorf("load loader summary counts: %w", err)
	}
	summary.TriggerCount = triggerCount
	summary.RunCount = runCount
	summary.EventCount = eventCount
	summary.LatestRunAt = parseStoredTime(latestRunAtRaw)
	return nil
}
