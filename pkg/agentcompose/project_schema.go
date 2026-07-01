package agentcompose

import (
	"context"
)

func (s *ConfigStore) ensureProjectSchema(ctx context.Context) error {
	return s.sqliteStore().EnsureProjectSchema(ctx)
}

func (s *ConfigStore) ensureManagedResourceColumns(ctx context.Context) error {
	return s.sqliteStore().EnsureManagedResourceColumns(ctx)
}
