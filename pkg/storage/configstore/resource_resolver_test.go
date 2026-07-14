package configstore

import (
	"context"
	"database/sql"
	"strings"
	"testing"

	"agent-compose/pkg/identity"
	domain "agent-compose/pkg/model"
)

func TestConfigStoreResolveStoredResourcesByNameAndIDPrefix(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()
	store := FromDB(db)
	if err := store.initSchema(ctx); err != nil {
		t.Fatalf("init schema: %v", err)
	}

	prefix := "abcdef123456"
	projectID := prefix + strings.Repeat("1", 52)
	agentID := prefix + strings.Repeat("2", 52)
	runID := prefix + strings.Repeat("3", 52)
	legacyRunID := identity.Prefix + prefix + strings.Repeat("4", 52)
	if _, err := db.ExecContext(ctx, `INSERT INTO project(id, name) VALUES(?, ?)`, projectID, "demo"); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO project_agent(id, name, short_id, project_id, agent_name, managed_agent_id) VALUES(?, ?, ?, ?, ?, ?)`,
		agentID, "reviewer", prefix, projectID, "reviewer", agentID); err != nil {
		t.Fatalf("insert project agent: %v", err)
	}
	for _, id := range []string{runID, legacyRunID} {
		if _, err := db.ExecContext(ctx, `INSERT INTO project_run(run_id, project_id, project_name, agent_name) VALUES(?, ?, ?, ?)`, id, projectID, "demo", "reviewer"); err != nil {
			t.Fatalf("insert project run %s: %v", id, err)
		}
	}
	if _, err := db.ExecContext(ctx, `INSERT INTO volumes(id, name, project_id) VALUES(?, ?, ?)`, prefix+"-volume-id", "cache-data", projectID); err != nil {
		t.Fatalf("insert volume: %v", err)
	}

	nameMatches, err := store.ResolveStoredResources(ctx, domain.ResourceResolveOptions{Ref: "reviewer", Kinds: []domain.ResourceKind{domain.ResourceKindAgent}})
	if err != nil {
		t.Fatalf("resolve agent name: %v", err)
	}
	if len(nameMatches) != 1 || nameMatches[0].ID != agentID || nameMatches[0].ProjectName != "demo" || nameMatches[0].MatchType != domain.ResourceMatchName {
		t.Fatalf("agent name matches = %#v", nameMatches)
	}

	prefixMatches, err := store.ResolveStoredResources(ctx, domain.ResourceResolveOptions{Ref: prefix})
	if err != nil {
		t.Fatalf("resolve id prefix: %v", err)
	}
	var projects, agents, runs, volumes int
	for _, match := range prefixMatches {
		switch match.Kind {
		case domain.ResourceKindProject:
			projects++
		case domain.ResourceKindAgent:
			agents++
		case domain.ResourceKindRun:
			runs++
		case domain.ResourceKindVolume:
			volumes++
		}
		if match.MatchType != domain.ResourceMatchIDPrefix {
			t.Fatalf("prefix match = %#v, want id_prefix", match)
		}
	}
	if projects != 1 || agents != 1 || runs != 2 || volumes != 0 {
		t.Fatalf("prefix match counts project/agent/run/volume = %d/%d/%d/%d; matches=%#v", projects, agents, runs, volumes, prefixMatches)
	}

	volumeMatches, err := store.ResolveStoredResources(ctx, domain.ResourceResolveOptions{Ref: "cache-data", Kinds: []domain.ResourceKind{domain.ResourceKindVolume}})
	if err != nil {
		t.Fatalf("resolve volume name: %v", err)
	}
	if len(volumeMatches) != 1 || volumeMatches[0].ID != "" || volumeMatches[0].InspectRef != "cache-data" {
		t.Fatalf("volume matches = %#v", volumeMatches)
	}
}

func TestConfigStoreResolveStoredResourcesExactID(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", ":memory:")
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer func() { _ = db.Close() }()
	store := FromDB(db)
	if err := store.initSchema(ctx); err != nil {
		t.Fatalf("init schema: %v", err)
	}
	projectID := strings.Repeat("f", 64)
	if _, err := db.ExecContext(ctx, `INSERT INTO project(id, name) VALUES(?, ?)`, projectID, "all-f"); err != nil {
		t.Fatalf("insert project: %v", err)
	}
	matches, err := store.ResolveStoredResources(ctx, domain.ResourceResolveOptions{Ref: identity.Prefix + projectID, Kinds: []domain.ResourceKind{domain.ResourceKindProject}})
	if err != nil {
		t.Fatalf("resolve exact project id: %v", err)
	}
	if len(matches) != 1 || matches[0].ID != projectID || matches[0].MatchType != domain.ResourceMatchID {
		t.Fatalf("exact matches = %#v", matches)
	}
}

func TestIdentityLookupRejectsUnknownColumn(t *testing.T) {
	clause, args, matchType, ok := identityLookup("external_input", strings.Repeat("a", 64))
	if ok || clause != "" || args != nil || matchType != "" {
		t.Fatalf("identityLookup accepted unknown column: clause=%q args=%v matchType=%q ok=%t", clause, args, matchType, ok)
	}
}
