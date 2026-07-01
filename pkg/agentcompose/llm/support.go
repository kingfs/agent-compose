package llm

import (
	appconfig "agent-compose/pkg/config"
	driverpkg "agent-compose/pkg/driver"
	"context"
	"database/sql"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	VMStatusStopped = "STOPPED"
	VMStatusFailed  = "FAILED"
)

type EnvVar struct {
	Name   string
	Value  string
	Secret bool
}

type SessionSummary struct {
	ID            string
	Driver        string
	VMStatus      string
	WorkspacePath string
}

type Session struct {
	Summary          SessionSummary
	EnvItems         []EnvVar
	RuntimeEnvItems  []EnvVar
	ProviderEnvItems []EnvVar
}

type GlobalEnvVar struct {
	Name  string
	Value string
}

type Store struct {
	db            *sql.DB
	listGlobalEnv func(context.Context) ([]GlobalEnvVar, error)
}

func NewStore(db *sql.DB, listGlobalEnv func(context.Context) ([]GlobalEnvVar, error)) *Store {
	return &Store{db: db, listGlobalEnv: listGlobalEnv}
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

func (s *Store) ListGlobalEnv(ctx context.Context) ([]GlobalEnvVar, error) {
	if s == nil || s.listGlobalEnv == nil {
		return nil, nil
	}
	return s.listGlobalEnv(ctx)
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return value
		}
	}
	return ""
}

func boolToInt(value bool) int {
	if value {
		return 1
	}
	return 0
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

func parseStoredUnixTimeAuto(value int64) time.Time {
	if value <= 0 {
		return time.Time{}
	}
	if value > 1_000_000_000_000 {
		return time.UnixMilli(value).UTC()
	}
	return time.Unix(value, 0).UTC()
}

func normalizeAgentKind(agent string) string {
	agent = strings.ToLower(strings.TrimSpace(agent))
	switch agent {
	case "":
		return ""
	case "codex":
		return "codex"
	case "claude", "claude-code", "claude_code":
		return "claude"
	case "gemini", "gemini-cli", "gemini_cli":
		return "gemini"
	case "opencode", "open-code", "open_code":
		return "opencode"
	default:
		return agent
	}
}

func hostSessionDir(session *Session) string {
	return filepath.Dir(session.Summary.WorkspacePath)
}

func hostSessionHome(session *Session) string {
	return filepath.Join(hostSessionDir(session), "home")
}

func guestSessionHome(config *appconfig.Config) string {
	return config.GuestHomePath
}

func requireStore(store *Store) error {
	if store == nil || store.db == nil {
		return fmt.Errorf("llm config store is unavailable")
	}
	return nil
}

func normalizeEnvItems(items []EnvVar) []EnvVar {
	if len(items) == 0 {
		return nil
	}
	result := make([]EnvVar, 0, len(items))
	for _, item := range items {
		name := strings.TrimSpace(item.Name)
		if name == "" {
			continue
		}
		item.Name = name
		result = append(result, item)
	}
	if len(result) == 0 {
		return nil
	}
	return result
}

var _ = driverpkg.LLMProviderKeyName
