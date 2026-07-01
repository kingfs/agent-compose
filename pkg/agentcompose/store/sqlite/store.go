package sqlite

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"strings"

	"agent-compose/pkg/agentcompose/event"
	"agent-compose/pkg/agentcompose/llm"
	"agent-compose/pkg/agentcompose/loader"
	"agent-compose/pkg/agentcompose/project"

	_ "modernc.org/sqlite"
)

type SchemaStep func(context.Context) error

type Store struct {
	db *sql.DB
}

func Open(dataRoot, dbAddr string) (*Store, error) {
	if err := os.MkdirAll(dataRoot, 0o755); err != nil {
		return nil, fmt.Errorf("create agent-compose data root: %w", err)
	}
	db, err := sql.Open("sqlite", dbAddr)
	if err != nil {
		return nil, fmt.Errorf("failed to open sqlite db: %w", err)
	}
	db.SetMaxOpenConns(1)
	db.SetMaxIdleConns(1)
	return NewStore(db), nil
}

func NewStore(db *sql.DB) *Store {
	return &Store{db: db}
}

func (s *Store) DB() *sql.DB {
	if s == nil {
		return nil
	}
	return s.db
}

func (s *Store) InitSchema(ctx context.Context, steps ...SchemaStep) error {
	statements := []string{
		`PRAGMA journal_mode = WAL;`,
		`PRAGMA foreign_keys = ON;`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("init agent-compose config schema: %w", err)
		}
	}
	for _, step := range steps {
		if step == nil {
			continue
		}
		if err := step(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) LoaderRepository() *loader.Store {
	return loader.NewStore(s.db)
}

func (s *Store) EventRepository() *event.Store {
	return event.NewStore(s.db)
}

func (s *Store) LLMRepository(listGlobalEnv func(context.Context) ([]llm.GlobalEnvVar, error)) *llm.Store {
	return llm.NewStore(s.db, listGlobalEnv)
}

func (s *Store) EnsureProjectSchema(ctx context.Context) error {
	for _, stmt := range project.SchemaStatements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create project schema: %w", err)
		}
	}
	if err := s.EnsureManagedResourceColumns(ctx); err != nil {
		return err
	}
	return nil
}

func (s *Store) EnsureManagedResourceColumns(ctx context.Context) error {
	for _, column := range project.ManagedResourceColumns {
		if err := EnsureColumn(ctx, s.db, column.Table, column.Name, column.Definition); err != nil {
			return fmt.Errorf("ensure %s managed column %s: %w", column.Table, column.Name, err)
		}
	}
	for _, stmt := range project.ManagedResourceIndexStatements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create managed resource index: %w", err)
		}
	}
	return nil
}

func EnsureColumn(ctx context.Context, db *sql.DB, table, column, definition string) error {
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

func (s *Store) TableColumnTypes(ctx context.Context, tableName string) (map[string]string, error) {
	trimmedTableName := strings.TrimSpace(tableName)
	if trimmedTableName == "" {
		return nil, fmt.Errorf("schema table name is required")
	}
	rows, err := s.db.QueryContext(ctx, fmt.Sprintf(`SELECT name, type FROM pragma_table_info('%s')`, strings.ReplaceAll(trimmedTableName, "'", "''")))
	if err != nil {
		return nil, fmt.Errorf("query schema for %s: %w", tableName, err)
	}
	defer func() { _ = rows.Close() }()

	columnTypes := make(map[string]string)
	for rows.Next() {
		var name string
		var columnType string
		if err := rows.Scan(&name, &columnType); err != nil {
			return nil, fmt.Errorf("scan schema for %s: %w", tableName, err)
		}
		columnTypes[strings.ToLower(strings.TrimSpace(name))] = strings.TrimSpace(columnType)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("iterate schema for %s: %w", tableName, err)
	}
	return columnTypes, nil
}

func IsIntegerColumnType(columnType string) bool {
	return strings.Contains(strings.ToUpper(strings.TrimSpace(columnType)), "INT")
}
