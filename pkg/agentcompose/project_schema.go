package agentcompose

import (
	"context"
	"fmt"

	projectpkg "agent-compose/pkg/agentcompose/project"
)

func (s *ConfigStore) ensureProjectSchema(ctx context.Context) error {
	for _, stmt := range projectpkg.SchemaStatements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create project schema: %w", err)
		}
	}
	if err := s.ensureManagedResourceColumns(ctx); err != nil {
		return err
	}
	return nil
}

func (s *ConfigStore) ensureManagedResourceColumns(ctx context.Context) error {
	for _, column := range projectpkg.ManagedResourceColumns {
		if err := ensureColumn(ctx, s.db, column.Table, column.Name, column.Definition); err != nil {
			return fmt.Errorf("ensure %s managed column %s: %w", column.Table, column.Name, err)
		}
	}
	for _, stmt := range projectpkg.ManagedResourceIndexStatements {
		if _, err := s.db.ExecContext(ctx, stmt); err != nil {
			return fmt.Errorf("create managed resource index: %w", err)
		}
	}
	return nil
}
