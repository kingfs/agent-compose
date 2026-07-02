package loader

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
)

func (s *Store) CreateLoaderRun(ctx context.Context, run LoaderRunSummary) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO loader_run(
        loader_id, run_id, trigger_id, trigger_kind, trigger_source, status, started_at, completed_at, duration_ms, error, result_json, payload_json, source_script_sha256, artifacts_dir
    ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(run.LoaderID),
		strings.TrimSpace(run.ID),
		strings.TrimSpace(run.TriggerID),
		strings.TrimSpace(run.TriggerKind),
		strings.TrimSpace(run.TriggerSource),
		normalizeLoaderRunStatus(run.Status),
		run.StartedAt.UTC().UnixMilli(),
		nonZeroTimeUnixMilli(run.CompletedAt),
		run.DurationMs,
		run.Error,
		run.ResultJSON,
		run.PayloadJSON,
		run.SourceScriptHash,
		strings.TrimSpace(run.ArtifactsDir),
	)
	if err != nil {
		return fmt.Errorf("insert loader run %s/%s: %w", run.LoaderID, run.ID, err)
	}
	return nil
}

func (s *Store) UpdateLoaderRun(ctx context.Context, run LoaderRunSummary) error {
	result, err := s.db.ExecContext(ctx, `UPDATE loader_run SET
        trigger_id = ?, trigger_kind = ?, trigger_source = ?, status = ?, started_at = ?, completed_at = ?, duration_ms = ?, error = ?, result_json = ?, payload_json = ?, source_script_sha256 = ?, artifacts_dir = ?
        WHERE loader_id = ? AND run_id = ?`,
		strings.TrimSpace(run.TriggerID),
		strings.TrimSpace(run.TriggerKind),
		strings.TrimSpace(run.TriggerSource),
		normalizeLoaderRunStatus(run.Status),
		run.StartedAt.UTC().UnixMilli(),
		nonZeroTimeUnixMilli(run.CompletedAt),
		run.DurationMs,
		run.Error,
		run.ResultJSON,
		run.PayloadJSON,
		run.SourceScriptHash,
		strings.TrimSpace(run.ArtifactsDir),
		strings.TrimSpace(run.LoaderID),
		strings.TrimSpace(run.ID),
	)
	if err != nil {
		return fmt.Errorf("update loader run %s/%s: %w", run.LoaderID, run.ID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("loader run %s/%s not found", run.LoaderID, run.ID)
	}
	return nil
}

func (s *Store) GetLoaderRun(ctx context.Context, loaderID, runID string) (LoaderRunSummary, error) {
	row := s.db.QueryRowContext(ctx, `SELECT loader_id, run_id, trigger_id, trigger_kind, trigger_source, status, started_at, completed_at, duration_ms, error, result_json, payload_json, source_script_sha256, artifacts_dir
        FROM loader_run WHERE loader_id = ? AND run_id = ?`, strings.TrimSpace(loaderID), strings.TrimSpace(runID))
	item, err := scanLoaderRun(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LoaderRunSummary{}, fmt.Errorf("loader run %s/%s not found: %w", loaderID, runID, err)
		}
		return LoaderRunSummary{}, err
	}
	return item, nil
}

func (s *Store) ListLoaderRuns(ctx context.Context, loaderID string, limit int) ([]LoaderRunSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT loader_id, run_id, trigger_id, trigger_kind, trigger_source, status, started_at, completed_at, duration_ms, error, result_json, payload_json, source_script_sha256, artifacts_dir
        FROM loader_run WHERE loader_id = ? ORDER BY started_at DESC, run_id DESC LIMIT ?`, strings.TrimSpace(loaderID), limit)
	if err != nil {
		return nil, fmt.Errorf("query loader runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]LoaderRunSummary, 0)
	for rows.Next() {
		item, err := scanLoaderRun(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate loader runs: %w", err)
	}
	return items, nil
}

func (s *Store) ListRecentLoaderRuns(ctx context.Context, limit int) ([]LoaderRunSummary, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := s.db.QueryContext(ctx, `SELECT loader_id, run_id, trigger_id, trigger_kind, trigger_source, status, started_at, completed_at, duration_ms, error, result_json, payload_json, source_script_sha256, artifacts_dir
        FROM loader_run ORDER BY started_at DESC, loader_id DESC, run_id DESC LIMIT ?`, limit)
	if err != nil {
		return nil, fmt.Errorf("query recent loader runs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]LoaderRunSummary, 0)
	for rows.Next() {
		item, err := scanLoaderRun(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate recent loader runs: %w", err)
	}
	return items, nil
}
