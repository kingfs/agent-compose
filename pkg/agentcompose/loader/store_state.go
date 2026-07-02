package loader

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) GetLoaderState(ctx context.Context, loaderID, key string) (string, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT value_json FROM loader_state WHERE loader_id = ? AND key = ?`, strings.TrimSpace(loaderID), strings.TrimSpace(key))
	var value string
	if err := row.Scan(&value); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return "", false, nil
		}
		return "", false, fmt.Errorf("query loader state: %w", err)
	}
	return value, true, nil
}

func (s *Store) SetLoaderState(ctx context.Context, loaderID, key, valueJSON string) error {
	loaderID = strings.TrimSpace(loaderID)
	key = strings.TrimSpace(key)
	if loaderID == "" || key == "" {
		return fmt.Errorf("loader state key is required")
	}
	_, err := s.db.ExecContext(ctx, `INSERT INTO loader_state(loader_id, key, value_json, updated_at) VALUES(?, ?, ?, ?)
        ON CONFLICT(loader_id, key) DO UPDATE SET value_json = excluded.value_json, updated_at = excluded.updated_at`, loaderID, key, strings.TrimSpace(valueJSON), time.Now().UTC().Unix())
	if err != nil {
		return fmt.Errorf("upsert loader state: %w", err)
	}
	return nil
}

func (s *Store) DeleteLoaderState(ctx context.Context, loaderID, key string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM loader_state WHERE loader_id = ? AND key = ?`, strings.TrimSpace(loaderID), strings.TrimSpace(key))
	if err != nil {
		return fmt.Errorf("delete loader state: %w", err)
	}
	return nil
}
