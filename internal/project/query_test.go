package project

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"testing"

	"agent-compose/internal/projecttypes"
)

func TestResolveProjectRefByID(t *testing.T) {
	ctx := context.Background()
	store := newQueryTestStore([]projecttypes.ProjectRecord{{ID: "project-1", Name: "demo"}})
	usecase := NewQueryUsecase(QueryUsecaseOptions{Store: store})

	project, err := usecase.ResolveProjectRef(ctx, ProjectRef{ProjectID: " project-1 "})
	if err != nil {
		t.Fatalf("ResolveProjectRef returned error: %v", err)
	}
	if project.ID != "project-1" {
		t.Fatalf("project.ID = %q, want project-1", project.ID)
	}
}

func TestResolveProjectRefByName(t *testing.T) {
	ctx := context.Background()
	store := newQueryTestStore([]projecttypes.ProjectRecord{{ID: "project-1", Name: "demo"}})
	usecase := NewQueryUsecase(QueryUsecaseOptions{Store: store})

	project, err := usecase.ResolveProjectRef(ctx, ProjectRef{Name: "demo"})
	if err != nil {
		t.Fatalf("ResolveProjectRef returned error: %v", err)
	}
	if project.ID != "project-1" {
		t.Fatalf("project.ID = %q, want project-1", project.ID)
	}
	if store.lastListOptions.Query != "demo" || store.lastListOptions.Limit != defaultResolveByNameLimit {
		t.Fatalf("ListProjects options = %+v", store.lastListOptions)
	}
}

func TestResolveProjectRefByNameAndSourcePath(t *testing.T) {
	ctx := context.Background()
	store := newQueryTestStore([]projecttypes.ProjectRecord{{ID: "stable-demo", Name: "demo", SourcePath: "/tmp/project.yml"}})
	usecase := NewQueryUsecase(QueryUsecaseOptions{
		Store: store,
		StableProjectID: func(name, sourcePath string) (string, error) {
			if name != "demo" || sourcePath != "/tmp/project.yml" {
				return "", fmt.Errorf("unexpected stable id input %q %q", name, sourcePath)
			}
			return "stable-demo", nil
		},
	})

	project, err := usecase.ResolveProjectRef(ctx, ProjectRef{Name: "demo", SourcePath: "/tmp/project.yml"})
	if err != nil {
		t.Fatalf("ResolveProjectRef returned error: %v", err)
	}
	if project.ID != "stable-demo" {
		t.Fatalf("project.ID = %q, want stable-demo", project.ID)
	}
}

func TestResolveProjectRefAmbiguousName(t *testing.T) {
	ctx := context.Background()
	store := newQueryTestStore([]projecttypes.ProjectRecord{
		{ID: "project-1", Name: "demo"},
		{ID: "project-2", Name: "demo"},
	})
	usecase := NewQueryUsecase(QueryUsecaseOptions{Store: store})

	_, err := usecase.ResolveProjectRef(ctx, ProjectRef{Name: "demo"})
	if err == nil {
		t.Fatalf("ResolveProjectRef returned nil error")
	}
	if got := ErrorKindOf(err); got != ErrorKindValidation {
		t.Fatalf("ErrorKindOf() = %q, want %q", got, ErrorKindValidation)
	}
}

func TestResolveProjectRefNotFound(t *testing.T) {
	ctx := context.Background()
	store := newQueryTestStore(nil)
	usecase := NewQueryUsecase(QueryUsecaseOptions{Store: store})

	_, err := usecase.ResolveProjectRef(ctx, ProjectRef{Name: "missing"})
	if err == nil {
		t.Fatalf("ResolveProjectRef returned nil error")
	}
	if got := ErrorKindOf(err); got != ErrorKindNotFound {
		t.Fatalf("ErrorKindOf() = %q, want %q", got, ErrorKindNotFound)
	}
	if !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("error does not wrap sql.ErrNoRows: %v", err)
	}
}

func TestRemoveProjectHistoryUnimplemented(t *testing.T) {
	ctx := context.Background()
	store := newQueryTestStore([]projecttypes.ProjectRecord{{ID: "project-1", Name: "demo"}})
	usecase := NewQueryUsecase(QueryUsecaseOptions{Store: store})

	_, err := usecase.RemoveProject(ctx, RemoveProjectRequest{Ref: ProjectRef{ProjectID: "project-1"}, RemoveHistory: true})
	if err == nil {
		t.Fatalf("RemoveProject returned nil error")
	}
	if got := ErrorKindOf(err); got != ErrorKindUnimplemented {
		t.Fatalf("ErrorKindOf() = %q, want %q", got, ErrorKindUnimplemented)
	}
}

type queryTestStore struct {
	projects        []projecttypes.ProjectRecord
	lastListOptions projecttypes.ProjectListOptions
}

func newQueryTestStore(projects []projecttypes.ProjectRecord) *queryTestStore {
	return &queryTestStore{projects: projects}
}

func (s *queryTestStore) GetProject(_ context.Context, projectID string) (projecttypes.ProjectRecord, error) {
	for _, project := range s.projects {
		if project.ID == projectID {
			return project, nil
		}
	}
	return projecttypes.ProjectRecord{}, fmt.Errorf("project %s not found: %w", projectID, sql.ErrNoRows)
}

func (s *queryTestStore) ListProjects(_ context.Context, options projecttypes.ProjectListOptions) (projecttypes.ProjectListResult, error) {
	s.lastListOptions = options
	return projecttypes.ProjectListResult{Projects: s.projects, TotalCount: len(s.projects)}, nil
}

func (s *queryTestStore) ListProjectAgents(_ context.Context, _ string) ([]projecttypes.ProjectAgentRecord, error) {
	return nil, nil
}

func (s *queryTestStore) ListProjectSchedulers(_ context.Context, _ string) ([]projecttypes.ProjectSchedulerRecord, error) {
	return nil, nil
}

func (s *queryTestStore) GetProjectRevision(_ context.Context, _ string, _ int64) (projecttypes.ProjectRevisionRecord, error) {
	return projecttypes.ProjectRevisionRecord{}, nil
}
