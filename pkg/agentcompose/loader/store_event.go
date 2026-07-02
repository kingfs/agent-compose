package loader

import (
	"context"
	"fmt"
	"strings"
)

func (s *Store) AddLoaderEvent(ctx context.Context, event LoaderEvent) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO loader_event(
        loader_id, event_id, run_id, trigger_id, type, level, message, payload_json, linked_session_id, linked_cell_id, linked_agent_session_id, created_at
    ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		strings.TrimSpace(event.LoaderID),
		strings.TrimSpace(event.ID),
		strings.TrimSpace(event.RunID),
		strings.TrimSpace(event.TriggerID),
		strings.TrimSpace(event.Type),
		strings.TrimSpace(event.Level),
		strings.TrimSpace(event.Message),
		strings.TrimSpace(event.PayloadJSON),
		strings.TrimSpace(event.LinkedSessionID),
		strings.TrimSpace(event.LinkedCellID),
		strings.TrimSpace(event.LinkedAgentSessionID),
		event.CreatedAt.UTC().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("insert loader event %s/%s: %w", event.LoaderID, event.ID, err)
	}
	return nil
}

func (s *Store) ListLoaderEvents(ctx context.Context, loaderID string, limit int) ([]LoaderEvent, error) {
	if limit <= 0 {
		limit = 100
	}
	rows, err := s.db.QueryContext(ctx, `SELECT loader_id, event_id, run_id, trigger_id, type, level, message, payload_json, linked_session_id, linked_cell_id, linked_agent_session_id, created_at
        FROM loader_event WHERE loader_id = ? ORDER BY created_at DESC, event_id DESC LIMIT ?`, strings.TrimSpace(loaderID), limit)
	if err != nil {
		return nil, fmt.Errorf("query loader events: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]LoaderEvent, 0)
	for rows.Next() {
		item, err := scanLoaderEvent(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate loader events: %w", err)
	}
	return items, nil
}
