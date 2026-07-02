package sqlite

import (
	"context"
	"testing"

	"agent-compose/pkg/agentcompose/project"
)

func ensureProjectTables(t *testing.T, store *Store) {
	t.Helper()
	for _, stmt := range project.SchemaStatements {
		if _, err := store.db.ExecContext(context.Background(), stmt); err != nil {
			t.Fatalf("create project table: %v", err)
		}
	}
}

func TestProjectRepositoryRevisionIdempotencyAndList(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	ensureProjectTables(t, store)
	repo := store.ProjectRepository()

	record, err := repo.UpsertProject(ctx, project.ProjectRecord{
		ID:         "project-demo",
		Name:       "demo",
		SourcePath: "/tmp/agent-compose.yml",
	})
	if err != nil {
		t.Fatalf("UpsertProject: %v", err)
	}
	if record.SourceJSON == "" || record.CreatedAt.IsZero() || record.UpdatedAt.IsZero() {
		t.Fatalf("project was not normalized and timestamped: %#v", record)
	}

	first, created, err := repo.SaveProjectRevision(ctx, project.RevisionRecord{
		ProjectID: record.ID,
		SpecHash:  "hash-1",
		SpecJSON:  `{"name":"demo"}`,
	})
	if err != nil {
		t.Fatalf("SaveProjectRevision first: %v", err)
	}
	if !created || first.Revision != 1 {
		t.Fatalf("first revision created=%v revision=%d, want created revision 1", created, first.Revision)
	}
	repeated, created, err := repo.SaveProjectRevision(ctx, project.RevisionRecord{
		ProjectID: record.ID,
		SpecHash:  "hash-1",
		SpecJSON:  `{"name":"demo"}`,
	})
	if err != nil {
		t.Fatalf("SaveProjectRevision repeated: %v", err)
	}
	if created || repeated.Revision != first.Revision {
		t.Fatalf("repeated revision created=%v revision=%d, want existing revision %d", created, repeated.Revision, first.Revision)
	}

	loaded, err := repo.GetProject(ctx, record.ID)
	if err != nil {
		t.Fatalf("GetProject: %v", err)
	}
	if loaded.CurrentRevision != 1 || loaded.SpecHash != "hash-1" {
		t.Fatalf("project revision pointer = %d/%q, want 1/hash-1", loaded.CurrentRevision, loaded.SpecHash)
	}
	listed, err := repo.ListProjects(ctx, project.ListOptions{Query: "demo", Limit: 1})
	if err != nil {
		t.Fatalf("ListProjects: %v", err)
	}
	if listed.TotalCount != 1 || len(listed.Projects) != 1 || listed.Projects[0].ID != record.ID {
		t.Fatalf("listed projects = %#v, want only %s", listed, record.ID)
	}
}

func TestProjectRepositoryRunFiltersAndSessionRelations(t *testing.T) {
	ctx := context.Background()
	store := newTestStore(t)
	ensureProjectTables(t, store)
	repo := store.ProjectRepository()

	record, err := repo.UpsertProject(ctx, project.ProjectRecord{ID: "project-runs", Name: "runs"})
	if err != nil {
		t.Fatalf("UpsertProject: %v", err)
	}
	for _, run := range []project.RunRecord{
		{RunID: "run-1", ProjectID: record.ID, ProjectName: record.Name, AgentName: "reviewer", Source: project.RunSourceManual, Status: project.RunStatusRunning, SessionID: "session-1"},
		{RunID: "run-2", ProjectID: record.ID, ProjectName: record.Name, AgentName: "worker", Source: project.RunSourceScheduler, Status: project.RunStatusSucceeded, SessionID: "session-2"},
		{RunID: "run-3", ProjectID: record.ID, ProjectName: record.Name, AgentName: "reviewer", Source: project.RunSourceManual, Status: project.RunStatusFailed},
	} {
		if _, err := repo.CreateProjectRun(ctx, run); err != nil {
			t.Fatalf("CreateProjectRun %s: %v", run.RunID, err)
		}
	}

	reviewerRuns, err := repo.ListProjectRunsByOptions(ctx, project.RunListOptions{ProjectID: record.ID, AgentName: "reviewer", Source: project.RunSourceManual})
	if err != nil {
		t.Fatalf("ListProjectRunsByOptions: %v", err)
	}
	if len(reviewerRuns) != 2 {
		t.Fatalf("reviewer manual runs = %#v, want 2", reviewerRuns)
	}
	sessionRuns, err := repo.ListProjectSessionRuns(ctx, project.SessionRelationFilter{ProjectID: record.ID, Statuses: []string{project.RunStatusRunning}})
	if err != nil {
		t.Fatalf("ListProjectSessionRuns: %v", err)
	}
	if len(sessionRuns) != 1 || sessionRuns[0].RunID != "run-1" {
		t.Fatalf("running session runs = %#v, want run-1", sessionRuns)
	}
	runsForSession, err := repo.ListProjectRunsForSession(ctx, "session-2")
	if err != nil {
		t.Fatalf("ListProjectRunsForSession: %v", err)
	}
	if len(runsForSession) != 1 || runsForSession[0].RunID != "run-2" {
		t.Fatalf("session-2 runs = %#v, want run-2", runsForSession)
	}
}
