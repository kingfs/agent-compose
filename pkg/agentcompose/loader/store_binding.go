package loader

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"
)

func (s *Store) GetLoaderBinding(ctx context.Context, loaderID string) (LoaderBinding, bool, error) {
	row := s.db.QueryRowContext(ctx, `SELECT loader_id, session_id, created_at, updated_at FROM loader_binding WHERE loader_id = ?`, strings.TrimSpace(loaderID))
	item, err := scanLoaderBinding(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return LoaderBinding{}, false, nil
		}
		return LoaderBinding{}, false, err
	}
	return item, true, nil
}

func (s *Store) UpsertLoaderBinding(ctx context.Context, binding LoaderBinding) error {
	binding.LoaderID = strings.TrimSpace(binding.LoaderID)
	binding.SessionID = strings.TrimSpace(binding.SessionID)
	if binding.LoaderID == "" || binding.SessionID == "" {
		return fmt.Errorf("loader binding requires loader id and session id")
	}
	now := time.Now().UTC()
	if binding.CreatedAt.IsZero() {
		binding.CreatedAt = now
	}
	binding.UpdatedAt = now
	_, err := s.db.ExecContext(ctx, `INSERT INTO loader_binding(loader_id, session_id, created_at, updated_at) VALUES(?, ?, ?, ?)
        ON CONFLICT(loader_id) DO UPDATE SET session_id = excluded.session_id, updated_at = excluded.updated_at`, binding.LoaderID, binding.SessionID, binding.CreatedAt.Unix(), binding.UpdatedAt.Unix())
	if err != nil {
		return fmt.Errorf("upsert loader binding: %w", err)
	}
	return nil
}
