package loader

import (
	"context"
	"database/sql"
	"fmt"
)

const storedUnixMillisecondThreshold int64 = 10_000_000_000

func (s *Store) EnsureSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS loader (
            id TEXT PRIMARY KEY,
            name TEXT NOT NULL,
            description TEXT NOT NULL DEFAULT '',
            runtime TEXT NOT NULL DEFAULT 'scheduler',
            script TEXT NOT NULL,
            workspace_id TEXT NOT NULL DEFAULT '',
            agent_id TEXT NOT NULL DEFAULT '',
            driver TEXT NOT NULL DEFAULT '',
            guest_image TEXT NOT NULL DEFAULT '',
            default_agent TEXT NOT NULL DEFAULT 'codex',
            session_policy TEXT NOT NULL DEFAULT 'sticky',
            concurrency_policy TEXT NOT NULL DEFAULT 'skip',
            capset_ids TEXT NOT NULL DEFAULT '[]',
            env_json TEXT NOT NULL DEFAULT '[]',
            enabled INTEGER NOT NULL DEFAULT 1,
            last_error TEXT NOT NULL DEFAULT '',
            created_at INTEGER NOT NULL DEFAULT (CAST(strftime('%s','now') AS INTEGER)),
            updated_at INTEGER NOT NULL DEFAULT (CAST(strftime('%s','now') AS INTEGER))
        );`,
		`CREATE TABLE IF NOT EXISTS loader_trigger (
            loader_id TEXT NOT NULL,
            trigger_id TEXT NOT NULL,
            kind TEXT NOT NULL,
            topic TEXT NOT NULL DEFAULT '',
            interval_ms INTEGER NOT NULL DEFAULT 0,
            enabled INTEGER NOT NULL DEFAULT 1,
            auto_id INTEGER NOT NULL DEFAULT 0,
            spec_json TEXT NOT NULL DEFAULT '{}',
            next_fire_at INTEGER NOT NULL DEFAULT 0,
            last_fired_at INTEGER NOT NULL DEFAULT 0,
            PRIMARY KEY(loader_id, trigger_id),
            FOREIGN KEY(loader_id) REFERENCES loader(id) ON DELETE CASCADE
        );`,
		`CREATE INDEX IF NOT EXISTS idx_loader_trigger_schedule ON loader_trigger(enabled, kind, next_fire_at);`,
		`CREATE TABLE IF NOT EXISTS loader_run (
            loader_id TEXT NOT NULL,
            run_id TEXT NOT NULL,
            trigger_id TEXT NOT NULL DEFAULT '',
            trigger_kind TEXT NOT NULL DEFAULT '',
            trigger_source TEXT NOT NULL DEFAULT '',
            status TEXT NOT NULL DEFAULT '',
            started_at INTEGER NOT NULL DEFAULT (CAST(strftime('%s','now') AS INTEGER)),
            completed_at INTEGER NOT NULL DEFAULT 0,
            duration_ms INTEGER NOT NULL DEFAULT 0,
            error TEXT NOT NULL DEFAULT '',
            result_json TEXT NOT NULL DEFAULT '',
            payload_json TEXT NOT NULL DEFAULT '',
            source_script_sha256 TEXT NOT NULL DEFAULT '',
            artifacts_dir TEXT NOT NULL DEFAULT '',
            PRIMARY KEY(loader_id, run_id),
            FOREIGN KEY(loader_id) REFERENCES loader(id) ON DELETE CASCADE
        );`,
		`CREATE INDEX IF NOT EXISTS idx_loader_run_started ON loader_run(loader_id, started_at DESC);`,
		`CREATE TABLE IF NOT EXISTS loader_event (
            loader_id TEXT NOT NULL,
            event_id TEXT NOT NULL,
            run_id TEXT NOT NULL DEFAULT '',
            trigger_id TEXT NOT NULL DEFAULT '',
            type TEXT NOT NULL,
            level TEXT NOT NULL DEFAULT 'info',
            message TEXT NOT NULL DEFAULT '',
            payload_json TEXT NOT NULL DEFAULT '',
            linked_session_id TEXT NOT NULL DEFAULT '',
            linked_cell_id TEXT NOT NULL DEFAULT '',
            linked_agent_session_id TEXT NOT NULL DEFAULT '',
            created_at INTEGER NOT NULL DEFAULT (CAST(strftime('%s','now') AS INTEGER)),
            PRIMARY KEY(loader_id, event_id),
            FOREIGN KEY(loader_id) REFERENCES loader(id) ON DELETE CASCADE
        );`,
		`CREATE INDEX IF NOT EXISTS idx_loader_event_created ON loader_event(loader_id, created_at DESC);`,
		`CREATE TABLE IF NOT EXISTS loader_state (
            loader_id TEXT NOT NULL,
            key TEXT NOT NULL,
            value_json TEXT NOT NULL,
            updated_at INTEGER NOT NULL DEFAULT (CAST(strftime('%s','now') AS INTEGER)),
            PRIMARY KEY(loader_id, key),
            FOREIGN KEY(loader_id) REFERENCES loader(id) ON DELETE CASCADE
        );`,
		`CREATE TABLE IF NOT EXISTS loader_binding (
            loader_id TEXT PRIMARY KEY,
            session_id TEXT NOT NULL,
            created_at INTEGER NOT NULL DEFAULT (CAST(strftime('%s','now') AS INTEGER)),
            updated_at INTEGER NOT NULL DEFAULT (CAST(strftime('%s','now') AS INTEGER))
        );`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create loader schema: %w", err)
		}
	}
	if err := s.ensureLoaderAgentIDColumn(ctx); err != nil {
		return err
	}
	if err := s.migrateLoaderTimestampPrecision(ctx); err != nil {
		return err
	}
	if err := s.ensureLoaderCapabilityColumn(ctx); err != nil {
		return err
	}
	if err := s.ensureLoaderManagedColumns(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Store) ensureLoaderManagedColumns(ctx context.Context) error {
	columns := []struct {
		name       string
		definition string
	}{
		{name: "managed_project_id", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "managed_project_revision", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "managed_agent_name", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "managed_scheduler_id", definition: "TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range columns {
		if err := ensureColumn(ctx, s.db, "loader", column.name, column.definition); err != nil {
			return fmt.Errorf("ensure loader managed column %s: %w", column.name, err)
		}
	}
	return nil
}

func (s *Store) ensureLoaderCapabilityColumn(ctx context.Context) error {
	columnTypes, err := tableColumnTypes(ctx, s.db, "loader")
	if err != nil {
		return err
	}
	if _, ok := columnTypes["capset_ids"]; ok {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE loader ADD COLUMN capset_ids TEXT NOT NULL DEFAULT '[]'`); err != nil {
		return fmt.Errorf("migrate loader capability column: %w", err)
	}
	return nil
}

func tableColumnTypes(ctx context.Context, db *sql.DB, table string) (map[string]string, error) {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	types := map[string]string{}
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return nil, err
		}
		types[name] = typ
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return types, nil
}

func (s *Store) ensureLoaderAgentIDColumn(ctx context.Context) error {
	if err := ensureColumn(ctx, s.db, "loader", "agent_id", "TEXT NOT NULL DEFAULT ''"); err != nil {
		return fmt.Errorf("ensure loader agent_id column: %w", err)
	}
	return nil
}

func ensureColumn(ctx context.Context, db *sql.DB, table, column, definition string) error {
	rows, err := db.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return err
	}
	defer func() { _ = rows.Close() }()
	for rows.Next() {
		var cid int
		var name string
		var typ string
		var notNull int
		var defaultValue any
		var pk int
		if err := rows.Scan(&cid, &name, &typ, &notNull, &defaultValue, &pk); err != nil {
			return err
		}
		if name == column {
			return nil
		}
	}
	if err := rows.Err(); err != nil {
		return err
	}
	_, err = db.ExecContext(ctx, "ALTER TABLE "+table+" ADD COLUMN "+column+" "+definition)
	return err
}

func (s *Store) migrateLoaderTimestampPrecision(ctx context.Context) error {
	statements := []string{
		fmt.Sprintf(`UPDATE loader_trigger SET next_fire_at = next_fire_at * 1000 WHERE next_fire_at > 0 AND next_fire_at < %d`, storedUnixMillisecondThreshold),
		fmt.Sprintf(`UPDATE loader_trigger SET last_fired_at = last_fired_at * 1000 WHERE last_fired_at > 0 AND last_fired_at < %d`, storedUnixMillisecondThreshold),
		fmt.Sprintf(`UPDATE loader_run SET started_at = started_at * 1000 WHERE started_at > 0 AND started_at < %d`, storedUnixMillisecondThreshold),
		fmt.Sprintf(`UPDATE loader_run SET completed_at = completed_at * 1000 WHERE completed_at > 0 AND completed_at < %d`, storedUnixMillisecondThreshold),
		fmt.Sprintf(`UPDATE loader_event SET created_at = created_at * 1000 WHERE created_at > 0 AND created_at < %d`, storedUnixMillisecondThreshold),
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate loader timestamp precision: %w", err)
		}
	}
	return nil
}
