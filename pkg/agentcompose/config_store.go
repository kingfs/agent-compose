package agentcompose

import (
	"agent-compose/pkg/agentcompose/configsvc"
	appconfig "agent-compose/pkg/config"
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/samber/do/v2"

	sqlitestore "agent-compose/pkg/agentcompose/store/sqlite"
)

const storedUnixMillisecondThreshold int64 = 10_000_000_000

type ConfigStore struct {
	db     *sql.DB
	sqlite *sqlitestore.Store
}

func NewConfigStore(di do.Injector) (*ConfigStore, error) {
	config := do.MustInvoke[*appconfig.Config](di)
	sqliteDB, err := sqlitestore.Open(config.DataRoot, config.DbAddr)
	if err != nil {
		return nil, err
	}
	store := &ConfigStore{db: sqliteDB.DB(), sqlite: sqliteDB}
	if err := store.initSchema(context.Background()); err != nil {
		_ = store.db.Close()
		return nil, err
	}
	return store, nil
}

func (s *ConfigStore) sqliteStore() *sqlitestore.Store {
	if s == nil {
		return nil
	}
	if s.sqlite == nil {
		s.sqlite = sqlitestore.NewStore(s.db)
	}
	return s.sqlite
}

func (s *ConfigStore) initSchema(ctx context.Context) error {
	return s.sqliteStore().InitSchema(ctx,
		s.ensureGlobalEnvSchema,
		s.ensureLLMSchema,
		s.ensureCapabilityGatewaySchema,
		s.ensureWorkspaceConfigSchema,
		s.ensureLoaderSchema,
		s.ensureAgentDefinitionSchema,
		s.ensureProjectSchema,
		s.ensureEventSchema,
	)
}

func (s *ConfigStore) ensureGlobalEnvSchema(ctx context.Context) error {
	return s.sqliteStore().EnsureGlobalEnvSchema(ctx)
}

func (s *ConfigStore) ensureWorkspaceConfigSchema(ctx context.Context) error {
	return s.sqliteStore().EnsureWorkspaceConfigSchema(ctx)
}

func (s *ConfigStore) ensureAgentDefinitionSchema(ctx context.Context) error {
	const createStmt = `CREATE TABLE IF NOT EXISTS agent_definition (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		description TEXT NOT NULL DEFAULT '',
		enabled INTEGER NOT NULL DEFAULT 1,
		deleted_at INTEGER NOT NULL DEFAULT 0,
		provider TEXT NOT NULL DEFAULT 'codex',
		model TEXT NOT NULL DEFAULT '',
		system_prompt TEXT NOT NULL DEFAULT '',
		driver TEXT NOT NULL DEFAULT '',
		guest_image TEXT NOT NULL DEFAULT '',
		workspace_id TEXT NOT NULL DEFAULT '',
		env_json TEXT NOT NULL DEFAULT '[]',
		config_json TEXT NOT NULL DEFAULT '{}',
		capset_ids TEXT NOT NULL DEFAULT '[]',
		created_at INTEGER NOT NULL,
		updated_at INTEGER NOT NULL
	);`
	if _, err := s.db.ExecContext(ctx, createStmt); err != nil {
		return fmt.Errorf("create agent definition schema: %w", err)
	}
	if err := ensureColumn(ctx, s.db, "agent_definition", "capset_ids", "TEXT NOT NULL DEFAULT '[]'"); err != nil {
		return fmt.Errorf("ensure agent definition capset_ids column: %w", err)
	}
	managedColumns := []struct {
		name       string
		definition string
	}{
		{name: "managed_project_id", definition: "TEXT NOT NULL DEFAULT ''"},
		{name: "managed_project_revision", definition: "INTEGER NOT NULL DEFAULT 0"},
		{name: "managed_agent_name", definition: "TEXT NOT NULL DEFAULT ''"},
	}
	for _, column := range managedColumns {
		if err := ensureColumn(ctx, s.db, "agent_definition", column.name, column.definition); err != nil {
			return fmt.Errorf("ensure agent definition managed column %s: %w", column.name, err)
		}
	}
	statements := []string{
		`CREATE INDEX IF NOT EXISTS idx_agent_definition_deleted_enabled ON agent_definition(deleted_at, enabled);`,
		`CREATE INDEX IF NOT EXISTS idx_agent_definition_workspace ON agent_definition(workspace_id);`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create agent definition index: %w", err)
		}
	}
	return nil
}

func (s *ConfigStore) tableColumnTypes(ctx context.Context, tableName string) (map[string]string, error) {
	return s.sqliteStore().TableColumnTypes(ctx, tableName)
}

func (s *ConfigStore) rebuildGlobalEnvTable(ctx context.Context) error {
	return s.sqliteStore().RebuildGlobalEnvTable(ctx)
}

func (s *ConfigStore) rebuildWorkspaceConfigTable(ctx context.Context) error {
	return s.sqliteStore().RebuildWorkspaceConfigTable(ctx)
}

func (s *ConfigStore) ListGlobalEnv(ctx context.Context) ([]SessionEnvVar, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT name, value, secret FROM global_env ORDER BY name ASC`)
	if err != nil {
		return nil, fmt.Errorf("query global env: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]SessionEnvVar, 0)
	for rows.Next() {
		var item SessionEnvVar
		var secret int
		if err := rows.Scan(&item.Name, &item.Value, &secret); err != nil {
			return nil, fmt.Errorf("scan global env: %w", err)
		}
		item.Secret = secret != 0
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate global env: %w", err)
	}
	return items, nil
}

func (s *ConfigStore) ReplaceGlobalEnv(ctx context.Context, items []SessionEnvVar) ([]SessionEnvVar, error) {
	normalized := normalizeEnvItems(items)
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("begin global env tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }()

	if _, err := tx.ExecContext(ctx, `DELETE FROM global_env`); err != nil {
		return nil, fmt.Errorf("reset global env: %w", err)
	}
	for _, item := range normalized {
		if _, err := tx.ExecContext(ctx, `INSERT INTO global_env(name, value, secret, updated_at) VALUES(?, ?, ?, ?)`, item.Name, item.Value, boolToInt(item.Secret), time.Now().UTC().Unix()); err != nil {
			return nil, fmt.Errorf("insert global env %s: %w", item.Name, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("commit global env tx: %w", err)
	}
	return normalized, nil
}

func (s *ConfigStore) ListWorkspaceConfigs(ctx context.Context) ([]WorkspaceConfig, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, type, config_json, comment, created_at, updated_at FROM workspace_config ORDER BY name ASC, id ASC`)
	if err != nil {
		return nil, fmt.Errorf("query workspace configs: %w", err)
	}
	defer func() { _ = rows.Close() }()

	items := make([]WorkspaceConfig, 0)
	for rows.Next() {
		item, err := scanWorkspaceConfig(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate workspace configs: %w", err)
	}
	return items, nil
}

func (s *ConfigStore) GetWorkspaceConfig(ctx context.Context, id string) (WorkspaceConfig, error) {
	row := s.db.QueryRowContext(ctx, `SELECT id, name, type, config_json, comment, created_at, updated_at FROM workspace_config WHERE id = ?`, strings.TrimSpace(id))
	item, err := scanWorkspaceConfig(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return WorkspaceConfig{}, fmt.Errorf("workspace config %s not found: %w", strings.TrimSpace(id), err)
		}
		return WorkspaceConfig{}, err
	}
	return item, nil
}

func (s *ConfigStore) CreateWorkspaceConfig(ctx context.Context, item WorkspaceConfig) (WorkspaceConfig, error) {
	normalized, err := normalizeWorkspaceConfig(item, true)
	if err != nil {
		return WorkspaceConfig{}, err
	}
	now := time.Now().UTC()
	normalized.CreatedAt = now
	normalized.UpdatedAt = now
	if _, err := s.db.ExecContext(ctx, `INSERT INTO workspace_config(id, name, type, config_json, comment, created_at, updated_at) VALUES(?, ?, ?, ?, ?, ?, ?)`, normalized.ID, normalized.Name, normalized.Type, normalized.ConfigJSON, normalized.Comment, normalized.CreatedAt.Unix(), normalized.UpdatedAt.Unix()); err != nil {
		return WorkspaceConfig{}, fmt.Errorf("insert workspace config %s: %w", normalized.ID, err)
	}
	return normalized, nil
}

func (s *ConfigStore) UpdateWorkspaceConfig(ctx context.Context, item WorkspaceConfig) (WorkspaceConfig, error) {
	normalized, err := normalizeWorkspaceConfig(item, false)
	if err != nil {
		return WorkspaceConfig{}, err
	}
	existing, err := s.GetWorkspaceConfig(ctx, normalized.ID)
	if err != nil {
		return WorkspaceConfig{}, err
	}
	normalized.CreatedAt = existing.CreatedAt
	normalized.UpdatedAt = time.Now().UTC()
	result, err := s.db.ExecContext(ctx, `UPDATE workspace_config SET name = ?, type = ?, config_json = ?, comment = ?, updated_at = ? WHERE id = ?`, normalized.Name, normalized.Type, normalized.ConfigJSON, normalized.Comment, normalized.UpdatedAt.Unix(), normalized.ID)
	if err != nil {
		return WorkspaceConfig{}, fmt.Errorf("update workspace config %s: %w", normalized.ID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return WorkspaceConfig{}, fmt.Errorf("workspace config %s not found", normalized.ID)
	}
	return normalized, nil
}

func (s *ConfigStore) DeleteWorkspaceConfig(ctx context.Context, id string) error {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return fmt.Errorf("workspace config id is required")
	}
	if err := s.ensureWorkspaceNotReferencedByAgent(ctx, trimmedID); err != nil {
		return err
	}
	result, err := s.db.ExecContext(ctx, `DELETE FROM workspace_config WHERE id = ?`, trimmedID)
	if err != nil {
		return fmt.Errorf("delete workspace config %s: %w", trimmedID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("workspace config %s not found", trimmedID)
	}
	return nil
}

func (s *ConfigStore) CreateAgentDefinition(ctx context.Context, item AgentDefinition) (AgentDefinition, error) {
	normalized, err := normalizeAgentDefinition(item, true)
	if err != nil {
		return AgentDefinition{}, err
	}
	now := time.Now().UTC()
	normalized.CreatedAt = now
	normalized.UpdatedAt = now
	normalized.DeletedAt = time.Time{}
	envJSON, err := encodeAgentEnvJSON(normalized.EnvItems)
	if err != nil {
		return AgentDefinition{}, err
	}
	capsetIDsJSON, err := encodeCapsetIDs(normalized.CapsetIDs)
	if err != nil {
		return AgentDefinition{}, err
	}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO agent_definition(
		id, name, description, enabled, deleted_at, provider, model, system_prompt, driver, guest_image, workspace_id, env_json, config_json, capset_ids,
		managed_project_id, managed_project_revision, managed_agent_name, created_at, updated_at
	) VALUES(?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		normalized.ID, normalized.Name, normalized.Description, boolToInt(normalized.Enabled), normalized.Provider, normalized.Model, normalized.SystemPrompt,
		normalized.Driver, normalized.GuestImage, normalized.WorkspaceID, envJSON, normalized.ConfigJSON, capsetIDsJSON,
		normalized.ManagedProjectID, normalized.ManagedProjectRevision, normalized.ManagedAgentName, normalized.CreatedAt.Unix(), normalized.UpdatedAt.Unix()); err != nil {
		return AgentDefinition{}, fmt.Errorf("insert agent definition %s: %w", normalized.ID, err)
	}
	return normalized, nil
}

func (s *ConfigStore) UpdateAgentDefinition(ctx context.Context, item AgentDefinition) (AgentDefinition, error) {
	normalized, err := normalizeAgentDefinition(item, true)
	if err != nil {
		return AgentDefinition{}, err
	}
	existing, err := s.GetAgentDefinition(ctx, normalized.ID)
	if err != nil {
		return AgentDefinition{}, err
	}
	if normalized.ManagedProjectID == "" && normalized.ManagedAgentName == "" && normalized.ManagedProjectRevision == 0 {
		normalized.ManagedProjectID = existing.ManagedProjectID
		normalized.ManagedProjectRevision = existing.ManagedProjectRevision
		normalized.ManagedAgentName = existing.ManagedAgentName
	}
	normalized.CreatedAt = existing.CreatedAt
	normalized.UpdatedAt = time.Now().UTC()
	normalized.DeletedAt = time.Time{}
	envJSON, err := encodeAgentEnvJSON(normalized.EnvItems)
	if err != nil {
		return AgentDefinition{}, err
	}
	capsetIDsJSON, err := encodeCapsetIDs(normalized.CapsetIDs)
	if err != nil {
		return AgentDefinition{}, err
	}
	result, err := s.db.ExecContext(ctx, `UPDATE agent_definition SET
		name = ?, description = ?, enabled = ?, provider = ?, model = ?, system_prompt = ?, driver = ?, guest_image = ?, workspace_id = ?, env_json = ?,
		config_json = ?, capset_ids = ?, managed_project_id = ?, managed_project_revision = ?, managed_agent_name = ?, updated_at = ?
		WHERE id = ? AND deleted_at = 0`,
		normalized.Name, normalized.Description, boolToInt(normalized.Enabled), normalized.Provider, normalized.Model, normalized.SystemPrompt,
		normalized.Driver, normalized.GuestImage, normalized.WorkspaceID, envJSON, normalized.ConfigJSON, capsetIDsJSON,
		normalized.ManagedProjectID, normalized.ManagedProjectRevision, normalized.ManagedAgentName, normalized.UpdatedAt.Unix(), normalized.ID)
	if err != nil {
		return AgentDefinition{}, fmt.Errorf("update agent definition %s: %w", normalized.ID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return AgentDefinition{}, fmt.Errorf("agent definition %s not found", normalized.ID)
	}
	return normalized, nil
}

func (s *ConfigStore) UpsertManagedAgentDefinition(ctx context.Context, item AgentDefinition) (AgentDefinition, error) {
	normalized, err := normalizeAgentDefinition(item, true)
	if err != nil {
		return AgentDefinition{}, err
	}
	if normalized.ManagedProjectID == "" || normalized.ManagedAgentName == "" {
		return AgentDefinition{}, fmt.Errorf("managed project id and managed agent name are required")
	}
	envJSON, err := encodeAgentEnvJSON(normalized.EnvItems)
	if err != nil {
		return AgentDefinition{}, err
	}
	capsetIDsJSON, err := encodeCapsetIDs(normalized.CapsetIDs)
	if err != nil {
		return AgentDefinition{}, err
	}
	now := time.Now().UTC()
	existing, found, err := s.getAgentDefinitionIfExists(ctx, normalized.ID, true)
	if err != nil {
		return AgentDefinition{}, err
	}
	if found {
		normalized.CreatedAt = existing.CreatedAt
		normalized.UpdatedAt = now
		normalized.DeletedAt = time.Time{}
		result, err := s.db.ExecContext(ctx, `UPDATE agent_definition SET
			name = ?, description = ?, enabled = ?, deleted_at = 0, provider = ?, model = ?, system_prompt = ?, driver = ?, guest_image = ?, workspace_id = ?,
			env_json = ?, config_json = ?, capset_ids = ?, managed_project_id = ?, managed_project_revision = ?, managed_agent_name = ?, updated_at = ?
			WHERE id = ?`,
			normalized.Name, normalized.Description, boolToInt(normalized.Enabled), normalized.Provider, normalized.Model, normalized.SystemPrompt,
			normalized.Driver, normalized.GuestImage, normalized.WorkspaceID, envJSON, normalized.ConfigJSON, capsetIDsJSON,
			normalized.ManagedProjectID, normalized.ManagedProjectRevision, normalized.ManagedAgentName, normalized.UpdatedAt.Unix(), normalized.ID)
		if err != nil {
			return AgentDefinition{}, fmt.Errorf("update managed agent definition %s: %w", normalized.ID, err)
		}
		if rows, _ := result.RowsAffected(); rows == 0 {
			return AgentDefinition{}, fmt.Errorf("managed agent definition %s not found", normalized.ID)
		}
		return s.GetAgentDefinition(ctx, normalized.ID)
	}

	normalized.CreatedAt = now
	normalized.UpdatedAt = now
	normalized.DeletedAt = time.Time{}
	if _, err := s.db.ExecContext(ctx, `INSERT INTO agent_definition(
		id, name, description, enabled, deleted_at, provider, model, system_prompt, driver, guest_image, workspace_id, env_json, config_json, capset_ids,
		managed_project_id, managed_project_revision, managed_agent_name, created_at, updated_at
	) VALUES(?, ?, ?, ?, 0, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		normalized.ID, normalized.Name, normalized.Description, boolToInt(normalized.Enabled), normalized.Provider, normalized.Model, normalized.SystemPrompt,
		normalized.Driver, normalized.GuestImage, normalized.WorkspaceID, envJSON, normalized.ConfigJSON, capsetIDsJSON,
		normalized.ManagedProjectID, normalized.ManagedProjectRevision, normalized.ManagedAgentName, normalized.CreatedAt.Unix(), normalized.UpdatedAt.Unix()); err != nil {
		return AgentDefinition{}, fmt.Errorf("insert managed agent definition %s: %w", normalized.ID, err)
	}
	return normalized, nil
}

func (s *ConfigStore) GetAgentDefinition(ctx context.Context, id string) (AgentDefinition, error) {
	return s.getAgentDefinition(ctx, id, false)
}

func (s *ConfigStore) GetAgentDefinitionIncludingDeleted(ctx context.Context, id string) (AgentDefinition, error) {
	return s.getAgentDefinition(ctx, id, true)
}

func (s *ConfigStore) getAgentDefinition(ctx context.Context, id string, includeDeleted bool) (AgentDefinition, error) {
	item, found, err := s.getAgentDefinitionIfExists(ctx, id, includeDeleted)
	if err != nil {
		return AgentDefinition{}, err
	}
	if !found {
		return AgentDefinition{}, fmt.Errorf("agent definition %s not found: %w", strings.TrimSpace(id), sql.ErrNoRows)
	}
	return item, nil
}

func (s *ConfigStore) getAgentDefinitionIfExists(ctx context.Context, id string, includeDeleted bool) (AgentDefinition, bool, error) {
	where := "id = ? AND deleted_at = 0"
	if includeDeleted {
		where = "id = ?"
	}
	row := s.db.QueryRowContext(ctx, `SELECT id, name, description, enabled, deleted_at, provider, model, system_prompt, driver, guest_image, workspace_id, env_json, config_json, capset_ids,
		managed_project_id, managed_project_revision, managed_agent_name, created_at, updated_at
		FROM agent_definition WHERE `+where, strings.TrimSpace(id))
	item, err := scanAgentDefinition(row.Scan)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return AgentDefinition{}, false, nil
		}
		return AgentDefinition{}, false, err
	}
	return item, true, nil
}

func (s *ConfigStore) ListAgentDefinitions(ctx context.Context, options AgentDefinitionListOptions) (AgentDefinitionListResult, error) {
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
	query := strings.ToLower(strings.TrimSpace(options.Query))
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, description, enabled, deleted_at, provider, model, system_prompt, driver, guest_image, workspace_id, env_json, config_json, capset_ids,
		managed_project_id, managed_project_revision, managed_agent_name, created_at, updated_at
		FROM agent_definition
		ORDER BY CASE WHEN deleted_at = 0 THEN 0 ELSE 1 END, updated_at DESC, created_at DESC, id ASC`)
	if err != nil {
		return AgentDefinitionListResult{}, fmt.Errorf("query agent definitions: %w", err)
	}
	defer func() { _ = rows.Close() }()

	matched := make([]AgentDefinition, 0)
	for rows.Next() {
		item, err := scanAgentDefinition(rows.Scan)
		if err != nil {
			return AgentDefinitionListResult{}, err
		}
		if !options.IncludeDisabled && (!item.Enabled || !item.DeletedAt.IsZero()) {
			continue
		}
		if query != "" && !agentMatchesQuery(item, query) {
			continue
		}
		matched = append(matched, item)
	}
	if err := rows.Err(); err != nil {
		return AgentDefinitionListResult{}, fmt.Errorf("iterate agent definitions: %w", err)
	}
	total := len(matched)
	end := offset + limit
	if offset > total {
		offset = total
	}
	if end > total {
		end = total
	}
	page := matched[offset:end]
	return AgentDefinitionListResult{
		Agents:     page,
		TotalCount: total,
		HasMore:    end < total,
		NextOffset: end,
	}, nil
}

func (s *ConfigStore) ListManagedAgentDefinitions(ctx context.Context, projectID string, includeDeleted bool) ([]AgentDefinition, error) {
	projectID = strings.TrimSpace(projectID)
	if projectID == "" {
		return nil, fmt.Errorf("project id is required")
	}
	where := "managed_project_id = ? AND deleted_at = 0"
	if includeDeleted {
		where = "managed_project_id = ?"
	}
	rows, err := s.db.QueryContext(ctx, `SELECT id, name, description, enabled, deleted_at, provider, model, system_prompt, driver, guest_image, workspace_id, env_json, config_json, capset_ids,
		managed_project_id, managed_project_revision, managed_agent_name, created_at, updated_at
		FROM agent_definition WHERE `+where+` ORDER BY managed_agent_name ASC, id ASC`, projectID)
	if err != nil {
		return nil, fmt.Errorf("query managed agent definitions %s: %w", projectID, err)
	}
	defer func() { _ = rows.Close() }()
	var items []AgentDefinition
	for rows.Next() {
		item, err := scanAgentDefinition(rows.Scan)
		if err != nil {
			return nil, err
		}
		items = append(items, item)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate managed agent definitions %s: %w", projectID, err)
	}
	return items, nil
}

func (s *ConfigStore) DeleteAgentDefinition(ctx context.Context, id string) error {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return fmt.Errorf("agent definition id is required")
	}
	now := time.Now().UTC().Unix()
	result, err := s.db.ExecContext(ctx, `UPDATE agent_definition SET enabled = 0, deleted_at = ?, updated_at = ? WHERE id = ? AND deleted_at = 0`, now, now, trimmedID)
	if err != nil {
		return fmt.Errorf("delete agent definition %s: %w", trimmedID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return fmt.Errorf("agent definition %s not found", trimmedID)
	}
	return nil
}

func (s *ConfigStore) SetAgentDefinitionEnabled(ctx context.Context, id string, enabled bool) (AgentDefinition, error) {
	trimmedID := strings.TrimSpace(id)
	if trimmedID == "" {
		return AgentDefinition{}, fmt.Errorf("agent definition id is required")
	}
	now := time.Now().UTC().Unix()
	result, err := s.db.ExecContext(ctx, `UPDATE agent_definition SET enabled = ?, updated_at = ? WHERE id = ? AND deleted_at = 0`, boolToInt(enabled), now, trimmedID)
	if err != nil {
		return AgentDefinition{}, fmt.Errorf("set agent definition enabled %s: %w", trimmedID, err)
	}
	if rows, _ := result.RowsAffected(); rows == 0 {
		return AgentDefinition{}, fmt.Errorf("agent definition %s not found", trimmedID)
	}
	return s.GetAgentDefinition(ctx, trimmedID)
}

func (s *ConfigStore) ensureWorkspaceNotReferencedByAgent(ctx context.Context, workspaceID string) error {
	trimmedID := strings.TrimSpace(workspaceID)
	if trimmedID == "" {
		return nil
	}
	var count int
	if err := s.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM agent_definition WHERE deleted_at = 0 AND workspace_id = ?`, trimmedID).Scan(&count); err != nil {
		return fmt.Errorf("query workspace agent references %s: %w", trimmedID, err)
	}
	if count > 0 {
		return fmt.Errorf("workspace config %s is referenced by %d agent definition(s)", trimmedID, count)
	}
	return nil
}

func normalizeEnvItems(items []SessionEnvVar) []SessionEnvVar {
	return configsvc.NormalizeEnvItems(items)
}

func encodeAgentEnvJSON(items []SessionEnvVar) (string, error) {
	normalized := normalizeEnvItems(items)
	if normalized == nil {
		normalized = []SessionEnvVar{}
	}
	data, err := json.Marshal(normalized)
	if err != nil {
		return "", fmt.Errorf("encode agent env items: %w", err)
	}
	return string(data), nil
}

func decodeAgentEnvJSON(raw string) ([]SessionEnvVar, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, nil
	}
	var items []SessionEnvVar
	if err := json.Unmarshal([]byte(raw), &items); err != nil {
		return nil, fmt.Errorf("decode agent env items: %w", err)
	}
	return normalizeEnvItems(items), nil
}

func scanAgentDefinition(scan func(dest ...any) error) (AgentDefinition, error) {
	var item AgentDefinition
	var enabled int
	var deletedAtRaw any
	var envJSON string
	var capsetIDsRaw string
	var createdAtRaw any
	var updatedAtRaw any
	if err := scan(&item.ID, &item.Name, &item.Description, &enabled, &deletedAtRaw, &item.Provider, &item.Model, &item.SystemPrompt,
		&item.Driver, &item.GuestImage, &item.WorkspaceID, &envJSON, &item.ConfigJSON, &capsetIDsRaw,
		&item.ManagedProjectID, &item.ManagedProjectRevision, &item.ManagedAgentName, &createdAtRaw, &updatedAtRaw); err != nil {
		return AgentDefinition{}, fmt.Errorf("scan agent definition: %w", err)
	}
	envItems, err := decodeAgentEnvJSON(envJSON)
	if err != nil {
		return AgentDefinition{}, err
	}
	item.Enabled = enabled != 0
	item.DeletedAt = parseStoredTime(deletedAtRaw)
	item.EnvItems = envItems
	item.CapsetIDs = decodeCapsetIDs(capsetIDsRaw)
	item.ManagedProjectID = strings.TrimSpace(item.ManagedProjectID)
	item.ManagedAgentName = strings.TrimSpace(item.ManagedAgentName)
	item.CreatedAt = parseStoredTime(createdAtRaw)
	item.UpdatedAt = parseStoredTime(updatedAtRaw)
	return item, nil
}

func agentMatchesQuery(item AgentDefinition, query string) bool {
	if query == "" {
		return true
	}
	fields := []string{item.Name, item.Description, item.Provider, item.ManagedProjectID, item.ManagedAgentName}
	for _, field := range fields {
		if strings.Contains(strings.ToLower(field), query) {
			return true
		}
	}
	return false
}

func mergeEnvItems(globalItems, sessionItems []SessionEnvVar) []SessionEnvVar {
	merged := make(map[string]SessionEnvVar, len(globalItems)+len(sessionItems))
	for _, item := range normalizeEnvItems(globalItems) {
		merged[item.Name] = item
	}
	for _, item := range normalizeEnvItems(sessionItems) {
		merged[item.Name] = item
	}
	if len(merged) == 0 {
		return nil
	}
	keys := make([]string, 0, len(merged))
	for key := range merged {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]SessionEnvVar, 0, len(keys))
	for _, key := range keys {
		result = append(result, merged[key])
	}
	return result
}

func normalizeWorkspaceConfig(item WorkspaceConfig, assignID bool) (WorkspaceConfig, error) {
	return configsvc.NormalizeWorkspaceConfig(item, assignID)
}

func scanWorkspaceConfig(scan func(dest ...any) error) (WorkspaceConfig, error) {
	var item WorkspaceConfig
	var createdAtRaw any
	var updatedAtRaw any
	if err := scan(&item.ID, &item.Name, &item.Type, &item.ConfigJSON, &item.Comment, &createdAtRaw, &updatedAtRaw); err != nil {
		return WorkspaceConfig{}, fmt.Errorf("scan workspace config: %w", err)
	}
	item.CreatedAt = parseStoredTime(createdAtRaw)
	item.UpdatedAt = parseStoredTime(updatedAtRaw)
	return item, nil
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

func parseStoredLoaderTriggerTime(value any) time.Time {
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
		return parseStoredLoaderTriggerTime(string(typed))
	case string:
		trimmed := strings.TrimSpace(typed)
		if trimmed == "" {
			return time.Time{}
		}
		if unixValue, err := strconv.ParseInt(trimmed, 10, 64); err == nil {
			return parseStoredUnixTimeAuto(unixValue)
		}
		return parseStoredTime(trimmed)
	default:
		return parseStoredTime(value)
	}
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

func normalizeSQLiteTimestampExpr(columnName string) string {
	return sqlitestore.NormalizeSQLiteTimestampExpr(columnName)
}

func isIntegerColumnType(columnType string) bool {
	return sqlitestore.IsIntegerColumnType(columnType)
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
}
