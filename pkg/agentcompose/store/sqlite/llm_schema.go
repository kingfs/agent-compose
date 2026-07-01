package sqlite

import (
	"context"
	"fmt"
)

func (s *Store) EnsureLLMSchema(ctx context.Context) error {
	statements := []string{
		`CREATE TABLE IF NOT EXISTS llm_provider (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			provider_type TEXT NOT NULL DEFAULT 'openai_compatible',
			default_wire_api TEXT NOT NULL DEFAULT 'responses',
			base_url TEXT NOT NULL,
			api_key TEXT NOT NULL DEFAULT '',
			auth_header TEXT NOT NULL DEFAULT 'Authorization',
			auth_scheme TEXT NOT NULL DEFAULT 'Bearer',
			headers_json TEXT NOT NULL DEFAULT '{}',
			weight INTEGER NOT NULL DEFAULT 10,
			enabled INTEGER NOT NULL DEFAULT 1,
			scope TEXT NOT NULL DEFAULT 'system',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS llm_model (
			id TEXT PRIMARY KEY,
			name TEXT NOT NULL,
			description TEXT NOT NULL DEFAULT '',
			default_model INTEGER NOT NULL DEFAULT 0,
			enabled INTEGER NOT NULL DEFAULT 1,
			scope TEXT NOT NULL DEFAULT 'system',
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL
		);`,
		`CREATE TABLE IF NOT EXISTS llm_provider_model (
			provider_id TEXT NOT NULL,
			model_id TEXT NOT NULL,
			wire_api TEXT NOT NULL DEFAULT '',
			weight INTEGER NOT NULL DEFAULT 10,
			PRIMARY KEY(provider_id, model_id)
		);`,
		`CREATE TABLE IF NOT EXISTS llm_facade_token (
			token_hash TEXT PRIMARY KEY,
			session_id TEXT NOT NULL,
			token_fingerprint TEXT NOT NULL,
			model TEXT NOT NULL DEFAULT '',
			provider_id TEXT NOT NULL DEFAULT '',
			wire_api TEXT NOT NULL DEFAULT '',
			source TEXT NOT NULL DEFAULT '',
			run_id TEXT NOT NULL DEFAULT '',
			issued_at INTEGER NOT NULL,
			expires_at INTEGER NOT NULL,
			revoked_at INTEGER NOT NULL DEFAULT 0
		);`,
		`CREATE INDEX IF NOT EXISTS idx_llm_facade_token_session ON llm_facade_token(session_id, revoked_at, expires_at);`,
	}
	for _, stmt := range statements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create llm schema: %w", err)
		}
	}
	if err := EnsureColumn(ctx, s.db, "llm_provider", "use_generic_responses_text_parts", "INTEGER NOT NULL DEFAULT 0"); err != nil {
		return fmt.Errorf("ensure llm provider generic responses text parts column: %w", err)
	}
	return nil
}
