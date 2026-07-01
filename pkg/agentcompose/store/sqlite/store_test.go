package sqlite

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "data.db"))
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = db.Close() })
	return NewStore(db)
}

func TestConfigSchemaHelpersMigrateLegacyTimeColumns(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	if _, err := store.db.ExecContext(ctx, `CREATE TABLE global_env (
		name TEXT PRIMARY KEY,
		value TEXT NOT NULL,
		secret INTEGER NOT NULL DEFAULT 0,
		updated_at TEXT NOT NULL DEFAULT ''
	);`); err != nil {
		t.Fatalf("create legacy global env: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO global_env(name, value, secret, updated_at) VALUES('A', 'B', 1, '2024-01-02T03:04:05Z')`); err != nil {
		t.Fatalf("insert legacy global env: %v", err)
	}
	if err := store.EnsureGlobalEnvSchema(ctx); err != nil {
		t.Fatalf("EnsureGlobalEnvSchema: %v", err)
	}
	columns, err := store.TableColumnTypes(ctx, "global_env")
	if err != nil {
		t.Fatalf("TableColumnTypes: %v", err)
	}
	if !IsIntegerColumnType(columns["updated_at"]) {
		t.Fatalf("updated_at column type = %q, want integer", columns["updated_at"])
	}
	var updatedAt int64
	if err := store.db.QueryRowContext(ctx, `SELECT updated_at FROM global_env WHERE name = 'A'`).Scan(&updatedAt); err != nil {
		t.Fatalf("query migrated global env: %v", err)
	}
	if updatedAt != 1704164645 {
		t.Fatalf("updated_at = %d, want 1704164645", updatedAt)
	}

	if _, err := store.db.ExecContext(ctx, `CREATE TABLE workspace_config (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		type TEXT NOT NULL,
		config_json TEXT NOT NULL DEFAULT '{}',
		comment TEXT NOT NULL DEFAULT '',
		created_at TEXT NOT NULL DEFAULT '',
		updated_at TEXT NOT NULL DEFAULT ''
	);`); err != nil {
		t.Fatalf("create legacy workspace config: %v", err)
	}
	if _, err := store.db.ExecContext(ctx, `INSERT INTO workspace_config(id, name, type, config_json, comment, created_at, updated_at)
		VALUES('w1', 'Workspace', 'local', '{}', '', '1704164645', '2024-01-02T03:04:06Z')`); err != nil {
		t.Fatalf("insert legacy workspace config: %v", err)
	}
	if err := store.EnsureWorkspaceConfigSchema(ctx); err != nil {
		t.Fatalf("EnsureWorkspaceConfigSchema: %v", err)
	}
	columns, err = store.TableColumnTypes(ctx, "workspace_config")
	if err != nil {
		t.Fatalf("TableColumnTypes workspace_config: %v", err)
	}
	if !IsIntegerColumnType(columns["created_at"]) || !IsIntegerColumnType(columns["updated_at"]) {
		t.Fatalf("workspace time column types = created_at:%q updated_at:%q, want integer", columns["created_at"], columns["updated_at"])
	}
	var createdAt, workspaceUpdatedAt int64
	if err := store.db.QueryRowContext(ctx, `SELECT created_at, updated_at FROM workspace_config WHERE id = 'w1'`).Scan(&createdAt, &workspaceUpdatedAt); err != nil {
		t.Fatalf("query migrated workspace config: %v", err)
	}
	if createdAt != 1704164645 || workspaceUpdatedAt != 1704164646 {
		t.Fatalf("workspace times = %d/%d, want 1704164645/1704164646", createdAt, workspaceUpdatedAt)
	}
}

func TestCapabilityGatewayRepository(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)

	if err := store.EnsureCapabilityGatewaySchema(ctx); err != nil {
		t.Fatalf("EnsureCapabilityGatewaySchema: %v", err)
	}
	empty, err := store.GetCapabilityGateway(ctx)
	if err != nil {
		t.Fatalf("GetCapabilityGateway empty: %v", err)
	}
	if empty != (CapabilityGatewaySettings{}) {
		t.Fatalf("empty settings = %#v", empty)
	}

	saved, err := store.SaveCapabilityGateway(ctx, CapabilityGatewaySettings{Addr: " http://127.0.0.1:9000 ", Token: " token "})
	if err != nil {
		t.Fatalf("SaveCapabilityGateway: %v", err)
	}
	if saved.Addr != "http://127.0.0.1:9000" || saved.Token != "token" {
		t.Fatalf("saved settings = %#v", saved)
	}
	loaded, err := store.GetCapabilityGateway(ctx)
	if err != nil {
		t.Fatalf("GetCapabilityGateway saved: %v", err)
	}
	if loaded != saved {
		t.Fatalf("loaded settings = %#v, want %#v", loaded, saved)
	}
}
