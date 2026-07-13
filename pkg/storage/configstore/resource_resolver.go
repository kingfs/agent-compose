package configstore

import (
	"context"
	"database/sql"
	"fmt"
	"strings"

	"agent-compose/pkg/identity"
	domain "agent-compose/pkg/model"
)

func (s *ConfigStore) ResolveStoredResources(ctx context.Context, options domain.ResourceResolveOptions) ([]domain.ResolvedResource, error) {
	if s == nil || s.db == nil {
		return nil, fmt.Errorf("config store is required")
	}
	ref := strings.TrimSpace(options.Ref)
	if ref == "" {
		return nil, fmt.Errorf("resource ref is required")
	}
	allowed := make(map[domain.ResourceKind]bool, len(options.Kinds))
	for _, kind := range options.Kinds {
		allowed[kind] = true
	}
	allows := func(kind domain.ResourceKind) bool { return len(allowed) == 0 || allowed[kind] }

	var result []domain.ResolvedResource
	if allows(domain.ResourceKindProject) {
		matches, err := s.resolveProjectResources(ctx, ref)
		if err != nil {
			return nil, err
		}
		result = append(result, matches...)
	}
	if allows(domain.ResourceKindAgent) {
		matches, err := s.resolveAgentResources(ctx, ref)
		if err != nil {
			return nil, err
		}
		result = append(result, matches...)
	}
	if allows(domain.ResourceKindRun) && identity.IsIDPrefix(ref) {
		matches, err := s.resolveRunResources(ctx, ref)
		if err != nil {
			return nil, err
		}
		result = append(result, matches...)
	}
	if allows(domain.ResourceKindVolume) {
		matches, err := s.resolveVolumeResources(ctx, ref)
		if err != nil {
			return nil, err
		}
		result = append(result, matches...)
	}
	return result, nil
}

func (s *ConfigStore) resolveProjectResources(ctx context.Context, ref string) ([]domain.ResolvedResource, error) {
	result, err := queryStoredResources(ctx, s.db.QueryContext, `SELECT id, name FROM project WHERE removed_at = 0 AND name = ?`, []any{ref}, func(values []string) domain.ResolvedResource {
		return domain.ResolvedResource{
			Kind:        domain.ResourceKindProject,
			MatchType:   domain.ResourceMatchName,
			ID:          values[0],
			ShortID:     identity.ShortID(values[0]),
			Name:        values[1],
			ProjectID:   values[0],
			ProjectName: values[1],
			InspectRef:  values[0],
		}
	})
	if err != nil {
		return nil, fmt.Errorf("resolve project name %q: %w", ref, err)
	}
	clause, args, matchType, ok := identityLookup("id", ref)
	if !ok {
		return result, nil
	}
	idMatches, err := queryStoredResources(ctx, s.db.QueryContext, `SELECT id, name FROM project WHERE removed_at = 0 AND `+clause, args, func(values []string) domain.ResolvedResource {
		return domain.ResolvedResource{
			Kind:        domain.ResourceKindProject,
			MatchType:   matchType,
			ID:          values[0],
			ShortID:     identity.ShortID(values[0]),
			Name:        values[1],
			ProjectID:   values[0],
			ProjectName: values[1],
			InspectRef:  values[0],
		}
	})
	if err != nil {
		return nil, fmt.Errorf("resolve project id %q: %w", ref, err)
	}
	return append(result, idMatches...), nil
}

func (s *ConfigStore) resolveAgentResources(ctx context.Context, ref string) ([]domain.ResolvedResource, error) {
	const selectAgent = `SELECT pa.id, pa.agent_name, pa.project_id, p.name FROM project_agent pa JOIN project p ON p.id = pa.project_id`
	result, err := queryStoredResources(ctx, s.db.QueryContext, selectAgent+` WHERE p.removed_at = 0 AND pa.agent_name = ?`, []any{ref}, func(values []string) domain.ResolvedResource {
		return domain.ResolvedResource{
			Kind:        domain.ResourceKindAgent,
			MatchType:   domain.ResourceMatchName,
			ID:          values[0],
			ShortID:     identity.ShortID(values[0]),
			Name:        values[1],
			ProjectID:   values[2],
			ProjectName: values[3],
			InspectRef:  values[1],
		}
	})
	if err != nil {
		return nil, fmt.Errorf("resolve agent name %q: %w", ref, err)
	}
	clause, args, matchType, ok := identityLookup("pa.id", ref)
	if !ok {
		return result, nil
	}
	idMatches, err := queryStoredResources(ctx, s.db.QueryContext, selectAgent+` WHERE p.removed_at = 0 AND `+clause, args, func(values []string) domain.ResolvedResource {
		return domain.ResolvedResource{
			Kind:        domain.ResourceKindAgent,
			MatchType:   matchType,
			ID:          values[0],
			ShortID:     identity.ShortID(values[0]),
			Name:        values[1],
			ProjectID:   values[2],
			ProjectName: values[3],
			InspectRef:  values[1],
		}
	})
	if err != nil {
		return nil, fmt.Errorf("resolve agent id %q: %w", ref, err)
	}
	return append(result, idMatches...), nil
}

func (s *ConfigStore) resolveRunResources(ctx context.Context, ref string) ([]domain.ResolvedResource, error) {
	clause, args, matchType, ok := identityLookup("run_id", ref)
	if !ok {
		return nil, nil
	}
	matches, err := queryStoredResources(ctx, s.db.QueryContext, `SELECT run_id, agent_name, project_id, project_name FROM project_run WHERE `+clause, args, func(values []string) domain.ResolvedResource {
		return domain.ResolvedResource{
			Kind:        domain.ResourceKindRun,
			MatchType:   matchType,
			ID:          values[0],
			ShortID:     identity.ShortID(values[0]),
			ProjectID:   values[2],
			ProjectName: values[3],
			InspectRef:  values[0],
		}
	})
	if err != nil {
		return nil, fmt.Errorf("resolve run id %q: %w", ref, err)
	}
	return matches, nil
}

func (s *ConfigStore) resolveVolumeResources(ctx context.Context, ref string) ([]domain.ResolvedResource, error) {
	const query = `SELECT v.name, v.project_id, COALESCE(p.name, '') FROM volumes v LEFT JOIN project p ON p.id = v.project_id WHERE v.name = ?`
	matches, err := queryStoredResources(ctx, s.db.QueryContext, query, []any{ref}, func(values []string) domain.ResolvedResource {
		return domain.ResolvedResource{
			Kind:        domain.ResourceKindVolume,
			MatchType:   domain.ResourceMatchName,
			Name:        values[0],
			ProjectID:   values[1],
			ProjectName: values[2],
			InspectRef:  values[0],
		}
	})
	if err != nil {
		return nil, fmt.Errorf("resolve volume name %q: %w", ref, err)
	}
	return matches, nil
}

func queryStoredResources(
	ctx context.Context,
	query func(context.Context, string, ...any) (*sql.Rows, error),
	statement string,
	args []any,
	build func([]string) domain.ResolvedResource,
) ([]domain.ResolvedResource, error) {
	rows, err := query(ctx, statement, args...)
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()
	columnCount, err := rows.Columns()
	if err != nil {
		return nil, err
	}
	result := make([]domain.ResolvedResource, 0)
	for rows.Next() {
		values := make([]string, len(columnCount))
		dest := make([]any, len(values))
		for index := range values {
			dest[index] = &values[index]
		}
		if err := rows.Scan(dest...); err != nil {
			return nil, err
		}
		result = append(result, build(values))
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	return result, nil
}

func identityLookup(column, ref string) (string, []any, domain.ResourceMatchType, bool) {
	ref = strings.TrimSpace(strings.ToLower(ref))
	ref = strings.TrimPrefix(ref, identity.Prefix)
	if !identity.IsIDPrefix(ref) {
		return "", nil, "", false
	}
	if identity.IsID(ref) {
		return `(` + column + ` = ? OR ` + column + ` = ?)`, []any{ref, identity.Prefix + ref}, domain.ResourceMatchID, true
	}
	upper := nextHexPrefix(ref)
	return `((` + column + ` >= ? AND ` + column + ` < ?) OR (` + column + ` >= ? AND ` + column + ` < ?))`,
		[]any{ref, upper, identity.Prefix + ref, identity.Prefix + upper}, domain.ResourceMatchIDPrefix, true
}

func nextHexPrefix(prefix string) string {
	value := []byte(prefix)
	for index := len(value) - 1; index >= 0; index-- {
		switch value[index] {
		case '0', '1', '2', '3', '4', '5', '6', '7', '8':
			value[index]++
			return string(value[:index+1])
		case '9':
			value[index] = 'a'
			return string(value[:index+1])
		case 'a', 'b', 'c', 'd', 'e':
			value[index]++
			return string(value[:index+1])
		case 'f':
			continue
		}
	}
	return "g"
}
