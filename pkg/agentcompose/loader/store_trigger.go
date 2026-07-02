package loader

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"
)

func (s *Store) ReplaceLoaderTriggers(ctx context.Context, loaderID string, triggers []LoaderTrigger) ([]LoaderTrigger, error) {
	loaderID = strings.TrimSpace(loaderID)
	if loaderID == "" {
		return nil, fmt.Errorf("loader id is required")
	}
	existing, err := s.listLoaderTriggers(ctx, loaderID)
	if err != nil {
		return nil, err
	}
	existingByID := make(map[string]LoaderTrigger, len(existing))
	for _, item := range existing {
		existingByID[item.ID] = item
	}

	normalized := make([]LoaderTrigger, 0, len(triggers))
	seen := make(map[string]struct{}, len(triggers))
	now := time.Now().UTC()
	for _, trigger := range triggers {
		current, err := normalizeLoaderTrigger(loaderID, trigger)
		if err != nil {
			return nil, err
		}
		if _, ok := seen[current.ID]; ok {
			return nil, fmt.Errorf("duplicate loader trigger id %q", current.ID)
		}
		seen[current.ID] = struct{}{}
		sameSchedule := false
		if previous, ok := existingByID[current.ID]; ok {
			current.Enabled = previous.Enabled
			current.LastFiredAt = previous.LastFiredAt
			if current.Kind == previous.Kind && current.Topic == previous.Topic && current.IntervalMs == previous.IntervalMs && current.SpecJSON == previous.SpecJSON {
				current.NextFireAt = previous.NextFireAt
				sameSchedule = true
			}
		}
		if !current.Enabled {
			current.NextFireAt = time.Time{}
		} else {
			switch current.Kind {
			case LoaderTriggerKindInterval:
				if current.NextFireAt.IsZero() {
					current.NextFireAt = loaderTriggerScheduledAt(now, current.IntervalMs)
				}
			case LoaderTriggerKindTimeout:
				if !sameSchedule {
					current.NextFireAt = loaderTriggerScheduledAt(now, current.IntervalMs)
				}
			case LoaderTriggerKindCron:
				if !sameSchedule || current.NextFireAt.IsZero() {
					nextFireAt, err := loaderTriggerNextFireAt(now, current, false)
					if err != nil {
						return nil, err
					}
					current.NextFireAt = nextFireAt
				}
			}
		}
		normalized = append(normalized, current)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin loader trigger tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM loader_trigger WHERE loader_id = ?`, loaderID); err != nil {
		return nil, fmt.Errorf("reset loader triggers: %w", err)
	}
	for _, trigger := range normalized {
		if _, err := tx.ExecContext(ctx, `INSERT INTO loader_trigger(
            loader_id, trigger_id, kind, topic, interval_ms, enabled, auto_id, spec_json, next_fire_at, last_fired_at
        ) VALUES(?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			trigger.LoaderID,
			trigger.ID,
			trigger.Kind,
			trigger.Topic,
			trigger.IntervalMs,
			boolToInt(trigger.Enabled),
			boolToInt(trigger.AutoID),
			trigger.SpecJSON,
			nonZeroTimeUnixMilli(trigger.NextFireAt),
			nonZeroTimeUnixMilli(trigger.LastFiredAt),
		); err != nil {
			return nil, fmt.Errorf("insert loader trigger %s/%s: %w", loaderID, trigger.ID, err)
		}
	}
	if _, err := tx.ExecContext(ctx, `UPDATE loader SET updated_at = ? WHERE id = ?`, time.Now().UTC().Unix(), loaderID); err != nil {
		return nil, fmt.Errorf("touch loader after trigger replace: %w", err)
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit loader trigger tx: %w", err)
	}
	sort.Slice(normalized, func(i, j int) bool {
		if normalized[i].Kind != normalized[j].Kind {
			return normalized[i].Kind < normalized[j].Kind
		}
		return normalized[i].ID < normalized[j].ID
	})
	return normalized, nil
}

func (s *Store) listLoaderTriggers(ctx context.Context, loaderID string) ([]LoaderTrigger, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT loader_id, trigger_id, kind, topic, interval_ms, enabled, auto_id, spec_json, next_fire_at, last_fired_at
        FROM loader_trigger WHERE loader_id = ? ORDER BY kind ASC, trigger_id ASC`, loaderID)
	if err != nil {
		return nil, fmt.Errorf("query loader triggers: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]LoaderTrigger, 0)
	for rows.Next() {
		item, err := scanLoaderTrigger(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate loader triggers: %w", err)
	}
	return items, nil
}

func (s *Store) SetLoaderEnabled(ctx context.Context, loaderID string, enabled bool) error {
	loaderID = strings.TrimSpace(loaderID)
	if loaderID == "" {
		return fmt.Errorf("loader id is required")
	}
	var (
		triggers []LoaderTrigger
		err      error
	)
	if enabled {
		triggers, err = s.listLoaderTriggers(ctx, loaderID)
		if err != nil {
			return err
		}
	}
	now := time.Now().UTC()
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin loader enable tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	result, err := tx.ExecContext(ctx, `UPDATE loader SET enabled = ?, updated_at = ? WHERE id = ?`, boolToInt(enabled), now.Unix(), loaderID)
	if err != nil {
		return fmt.Errorf("update loader enabled state: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("loader %s not found", loaderID)
	}
	if enabled {
		for _, trigger := range triggers {
			if !trigger.Enabled || !loaderTriggerUsesSchedule(trigger.Kind) {
				continue
			}
			nextFireAt, err := loaderTriggerNextFireAt(now, trigger, false)
			if err != nil {
				return fmt.Errorf("schedule loader trigger %s/%s: %w", loaderID, trigger.ID, err)
			}
			if _, err := tx.ExecContext(ctx, `UPDATE loader_trigger SET next_fire_at = ? WHERE loader_id = ? AND trigger_id = ?`, nonZeroTimeUnixMilli(nextFireAt), loaderID, trigger.ID); err != nil {
				return fmt.Errorf("schedule loader trigger %s/%s: %w", loaderID, trigger.ID, err)
			}
		}
	} else {
		if _, err := tx.ExecContext(ctx, `UPDATE loader_trigger SET next_fire_at = 0 WHERE loader_id = ? AND kind IN (?, ?, ?)`, loaderID, LoaderTriggerKindInterval, LoaderTriggerKindTimeout, LoaderTriggerKindCron); err != nil {
			return fmt.Errorf("pause loader scheduled triggers: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit loader enable tx: %w", err)
	}
	return nil
}

func (s *Store) SetLoaderTriggerEnabled(ctx context.Context, loaderID, triggerID string, enabled bool) error {
	loaderID = strings.TrimSpace(loaderID)
	triggerID = strings.TrimSpace(triggerID)
	if loaderID == "" || triggerID == "" {
		return fmt.Errorf("loader trigger id is required")
	}
	row := s.db.QueryRowContext(ctx, `SELECT loader_id, trigger_id, kind, topic, interval_ms, enabled, auto_id, spec_json, next_fire_at, last_fired_at
        FROM loader_trigger WHERE loader_id = ? AND trigger_id = ?`, loaderID, triggerID)
	trigger, err := scanLoaderTrigger(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return fmt.Errorf("loader trigger %s/%s not found: %w", loaderID, triggerID, err)
		}
		return err
	}
	nextFireAt := int64(0)
	if enabled && loaderTriggerUsesSchedule(trigger.Kind) {
		scheduledAt, err := loaderTriggerNextFireAt(time.Now().UTC(), trigger, false)
		if err != nil {
			return fmt.Errorf("schedule loader trigger %s/%s: %w", loaderID, triggerID, err)
		}
		nextFireAt = nonZeroTimeUnixMilli(scheduledAt)
	}
	result, err := s.db.ExecContext(ctx, `UPDATE loader_trigger SET enabled = ?, next_fire_at = ? WHERE loader_id = ? AND trigger_id = ?`, boolToInt(enabled), nextFireAt, loaderID, triggerID)
	if err != nil {
		return fmt.Errorf("update loader trigger enabled state: %w", err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("loader trigger %s/%s not found", loaderID, triggerID)
	}
	_, _ = s.db.ExecContext(ctx, `UPDATE loader SET updated_at = ? WHERE id = ?`, time.Now().UTC().Unix(), loaderID)
	return nil
}

func (s *Store) UpdateLoaderLastError(ctx context.Context, loaderID, lastError string) error {
	loaderID = strings.TrimSpace(loaderID)
	if loaderID == "" {
		return fmt.Errorf("loader id is required")
	}
	_, err := s.db.ExecContext(ctx, `UPDATE loader SET last_error = ?, updated_at = ? WHERE id = ?`, strings.TrimSpace(lastError), time.Now().UTC().Unix(), loaderID)
	if err != nil {
		return fmt.Errorf("update loader last error: %w", err)
	}
	return nil
}

func (s *Store) MarkLoaderTriggerFired(ctx context.Context, loaderID, triggerID string, lastFiredAt, nextFireAt time.Time) error {
	_, err := s.db.ExecContext(ctx, `UPDATE loader_trigger SET last_fired_at = ?, next_fire_at = ? WHERE loader_id = ? AND trigger_id = ?`, nonZeroTimeUnixMilli(lastFiredAt), nonZeroTimeUnixMilli(nextFireAt), strings.TrimSpace(loaderID), strings.TrimSpace(triggerID))
	if err != nil {
		return fmt.Errorf("update loader trigger fire state: %w", err)
	}
	return nil
}
