package sqlite

import (
	"context"
	"fmt"
)

func (s *Store) EnsureGlobalEnvSchema(ctx context.Context) error {
	const createStmt = `CREATE TABLE IF NOT EXISTS global_env (
		name TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		secret INTEGER NOT NULL DEFAULT 0,
		updated_at INTEGER NOT NULL DEFAULT (CAST(strftime('%s','now') AS INTEGER))
	);`
	if _, err := s.db.ExecContext(ctx, createStmt); err != nil {
		return fmt.Errorf("create global env schema: %w", err)
	}
	columnTypes, err := s.TableColumnTypes(ctx, "global_env")
	if err != nil {
		return err
	}
	if IsIntegerColumnType(columnTypes["updated_at"]) {
		return nil
	}
	return s.RebuildGlobalEnvTable(ctx)
}

func (s *Store) EnsureWorkspaceConfigSchema(ctx context.Context) error {
	const createStmt = `CREATE TABLE IF NOT EXISTS workspace_config (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		config_json TEXT NOT NULL DEFAULT '{}',
		comment TEXT NOT NULL DEFAULT '',
		created_at INTEGER NOT NULL DEFAULT (CAST(strftime('%s','now') AS INTEGER)),
		updated_at INTEGER NOT NULL DEFAULT (CAST(strftime('%s','now') AS INTEGER))
	);`
	if _, err := s.db.ExecContext(ctx, createStmt); err != nil {
		return fmt.Errorf("create workspace config schema: %w", err)
	}
	columnTypes, err := s.TableColumnTypes(ctx, "workspace_config")
	if err != nil {
		return err
	}
	if IsIntegerColumnType(columnTypes["created_at"]) && IsIntegerColumnType(columnTypes["updated_at"]) {
		return nil
	}
	return s.RebuildWorkspaceConfigTable(ctx)
}

func (s *Store) RebuildGlobalEnvTable(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin global env migration tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	updatedAtExpr := NormalizeSQLiteTimestampExpr("updated_at")
	statements := []string{
		`DROP TABLE IF EXISTS global_env_legacy;`,
		`ALTER TABLE global_env RENAME TO global_env_legacy;`,
		`CREATE TABLE global_env (
			name TEXT PRIMARY KEY,
			value TEXT NOT NULL,
			secret INTEGER NOT NULL DEFAULT 0,
			updated_at INTEGER NOT NULL DEFAULT (CAST(strftime('%s','now') AS INTEGER))
		);`,
		fmt.Sprintf(`INSERT INTO global_env(name, value, secret, updated_at)
			SELECT name, value, secret, %s FROM global_env_legacy;`, updatedAtExpr),
		`DROP TABLE global_env_legacy;`,
	}
	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate global env schema: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit global env migration tx: %w", err)
	}
	return nil
}

func (s *Store) RebuildWorkspaceConfigTable(ctx context.Context) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return fmt.Errorf("begin workspace config migration tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	createdAtExpr := NormalizeSQLiteTimestampExpr("created_at")
	updatedAtExpr := NormalizeSQLiteTimestampExpr("updated_at")
	statements := []string{
		`DROP TABLE IF EXISTS workspace_config_legacy;`,
		`ALTER TABLE workspace_config RENAME TO workspace_config_legacy;`,
		`CREATE TABLE workspace_config (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			type TEXT NOT NULL,
			config_json TEXT NOT NULL DEFAULT '{}',
			comment TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL DEFAULT (CAST(strftime('%s','now') AS INTEGER)),
			updated_at INTEGER NOT NULL DEFAULT (CAST(strftime('%s','now') AS INTEGER))
		);`,
		fmt.Sprintf(`INSERT INTO workspace_config(id, name, type, config_json, comment, created_at, updated_at)
			SELECT id, name, type, config_json, comment, %s, %s FROM workspace_config_legacy;`, createdAtExpr, updatedAtExpr),
		`DROP TABLE workspace_config_legacy;`,
	}
	for _, stmt := range statements {
		if _, err := tx.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("migrate workspace config schema: %w", err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit workspace config migration tx: %w", err)
	}
	return nil
}

func NormalizeSQLiteTimestampExpr(columnName string) string {
	return fmt.Sprintf(`CASE
		WHEN trim(COALESCE(%[1]s, '')) = '' THEN CAST(strftime('%%s','now') AS INTEGER)
		WHEN trim(COALESCE(%[1]s, '')) NOT GLOB '*[^0-9]*' THEN CAST(%[1]s AS INTEGER)
		ELSE COALESCE(CAST(strftime('%%s', %[1]s) AS INTEGER), CAST(strftime('%%s','now') AS INTEGER))
	END`, columnName)
}
