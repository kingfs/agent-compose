package project

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"

	sqlitestore "agent-compose/internal/persistence/sqlite"
)

const defaultResolveByNameLimit = 200

type ProjectRef struct {
	ProjectID  string
	Name       string
	SourcePath string
}

type Store interface {
	GetProject(ctx context.Context, projectID string) (sqlitestore.ProjectRecord, error)
	ListProjects(ctx context.Context, options sqlitestore.ProjectListOptions) (sqlitestore.ProjectListResult, error)
	ListProjectAgents(ctx context.Context, projectID string) ([]sqlitestore.ProjectAgentRecord, error)
	ListProjectSchedulers(ctx context.Context, projectID string) ([]sqlitestore.ProjectSchedulerRecord, error)
	GetProjectRevision(ctx context.Context, projectID string, revision int64) (sqlitestore.ProjectRevisionRecord, error)
}

type Runtime interface {
	DownProject(ctx context.Context, project sqlitestore.ProjectRecord) ([]Change, error)
}

type StableProjectIDFunc func(name, sourcePath string) (string, error)

type QueryUsecase struct {
	store           Store
	runtime         Runtime
	stableProjectID StableProjectIDFunc
}

type QueryUsecaseOptions struct {
	Store           Store
	Runtime         Runtime
	StableProjectID StableProjectIDFunc
}

func NewQueryUsecase(options QueryUsecaseOptions) *QueryUsecase {
	return &QueryUsecase{
		store:           options.Store,
		runtime:         options.Runtime,
		stableProjectID: options.StableProjectID,
	}
}

type GetProjectRequest struct {
	Ref         ProjectRef
	IncludeSpec bool
}

type GetProjectResult struct {
	Project          sqlitestore.ProjectRecord
	Agents           []sqlitestore.ProjectAgentRecord
	Schedulers       []sqlitestore.ProjectSchedulerRecord
	RevisionSpecJSON string
}

type ListProjectsRequest struct {
	Query          string
	IncludeRemoved bool
	Offset         int
	Limit          int
}

type ListProjectsResult = sqlitestore.ProjectListResult

type RemoveProjectRequest struct {
	Ref           ProjectRef
	RemoveHistory bool
}

type RemoveProjectResult struct {
	Project    sqlitestore.ProjectRecord
	Agents     []sqlitestore.ProjectAgentRecord
	Schedulers []sqlitestore.ProjectSchedulerRecord
	Changes    []Change
}

func (u *QueryUsecase) GetProject(ctx context.Context, req GetProjectRequest) (GetProjectResult, error) {
	if err := u.requireStore(); err != nil {
		return GetProjectResult{}, err
	}
	project, err := u.ResolveProjectRef(ctx, req.Ref)
	if err != nil {
		return GetProjectResult{}, err
	}
	agents, schedulers, err := u.loadProjectChildren(ctx, project.ID)
	if err != nil {
		return GetProjectResult{}, err
	}
	result := GetProjectResult{
		Project:    project,
		Agents:     agents,
		Schedulers: schedulers,
	}
	if req.IncludeSpec && project.CurrentRevision > 0 {
		revision, err := u.store.GetProjectRevision(ctx, project.ID, project.CurrentRevision)
		if err != nil {
			return GetProjectResult{}, storageError("get project revision", err)
		}
		result.RevisionSpecJSON = revision.SpecJSON
	}
	return result, nil
}

func (u *QueryUsecase) ListProjects(ctx context.Context, req ListProjectsRequest) (ListProjectsResult, error) {
	if err := u.requireStore(); err != nil {
		return ListProjectsResult{}, err
	}
	result, err := u.store.ListProjects(ctx, sqlitestore.ProjectListOptions{
		Query:          req.Query,
		IncludeRemoved: req.IncludeRemoved,
		Offset:         req.Offset,
		Limit:          req.Limit,
	})
	if err != nil {
		return ListProjectsResult{}, storageError("list projects", err)
	}
	return result, nil
}

func (u *QueryUsecase) RemoveProject(ctx context.Context, req RemoveProjectRequest) (RemoveProjectResult, error) {
	if err := u.requireStore(); err != nil {
		return RemoveProjectResult{}, err
	}
	if req.RemoveHistory {
		return RemoveProjectResult{}, NewError(ErrorKindUnimplemented, "project history removal is not implemented", nil)
	}
	project, err := u.ResolveProjectRef(ctx, req.Ref)
	if err != nil {
		return RemoveProjectResult{}, err
	}
	var changes []Change
	if u.runtime != nil {
		changes, err = u.runtime.DownProject(ctx, project)
		if err != nil {
			return RemoveProjectResult{}, NewError(ErrorKindRuntime, fmt.Sprintf("down project %s", project.Name), err)
		}
	}
	agents, schedulers, err := u.loadProjectChildren(ctx, project.ID)
	if err != nil {
		return RemoveProjectResult{}, err
	}
	return RemoveProjectResult{
		Project:    project,
		Agents:     agents,
		Schedulers: schedulers,
		Changes:    changes,
	}, nil
}

func (u *QueryUsecase) ResolveProjectRef(ctx context.Context, ref ProjectRef) (sqlitestore.ProjectRecord, error) {
	if err := u.requireStore(); err != nil {
		return sqlitestore.ProjectRecord{}, err
	}
	if projectID := strings.TrimSpace(ref.ProjectID); projectID != "" {
		project, err := u.store.GetProject(ctx, projectID)
		if err != nil {
			return sqlitestore.ProjectRecord{}, projectLookupError("project "+projectID, err)
		}
		return project, nil
	}
	name := strings.TrimSpace(ref.Name)
	sourcePath := strings.TrimSpace(ref.SourcePath)
	if name != "" && sourcePath != "" {
		stableProjectID := u.stableProjectID
		if stableProjectID == nil {
			return sqlitestore.ProjectRecord{}, NewError(ErrorKindStorage, "stable project id resolver is required", nil)
		}
		projectID, err := stableProjectID(name, sourcePath)
		if err != nil {
			return sqlitestore.ProjectRecord{}, NewError(ErrorKindValidation, err.Error(), err)
		}
		project, err := u.store.GetProject(ctx, projectID)
		if err != nil {
			return sqlitestore.ProjectRecord{}, projectLookupError("project "+name, err)
		}
		return project, nil
	}
	if name == "" {
		return sqlitestore.ProjectRecord{}, NewError(ErrorKindValidation, "project id or name is required", nil)
	}
	result, err := u.store.ListProjects(ctx, sqlitestore.ProjectListOptions{Query: name, Limit: defaultResolveByNameLimit})
	if err != nil {
		return sqlitestore.ProjectRecord{}, storageError("list projects by name", err)
	}
	matches := make([]sqlitestore.ProjectRecord, 0, 1)
	for _, project := range result.Projects {
		if project.Name == name {
			matches = append(matches, project)
		}
	}
	if len(matches) == 0 {
		return sqlitestore.ProjectRecord{}, NewError(ErrorKindNotFound, fmt.Sprintf("project %s not found", name), sql.ErrNoRows)
	}
	if len(matches) > 1 {
		return sqlitestore.ProjectRecord{}, NewError(ErrorKindValidation, fmt.Sprintf("project name %s is ambiguous; use project_id or source_path", name), nil)
	}
	return matches[0], nil
}

func (u *QueryUsecase) loadProjectChildren(ctx context.Context, projectID string) ([]sqlitestore.ProjectAgentRecord, []sqlitestore.ProjectSchedulerRecord, error) {
	agents, err := u.store.ListProjectAgents(ctx, projectID)
	if err != nil {
		return nil, nil, storageError("list project agents", err)
	}
	schedulers, err := u.store.ListProjectSchedulers(ctx, projectID)
	if err != nil {
		return nil, nil, storageError("list project schedulers", err)
	}
	return agents, schedulers, nil
}

func (u *QueryUsecase) requireStore() error {
	if u == nil || u.store == nil {
		return NewError(ErrorKindStorage, "config store is required", nil)
	}
	return nil
}

func projectLookupError(message string, err error) error {
	if errors.Is(err, sql.ErrNoRows) {
		return NewError(ErrorKindNotFound, message+" not found", err)
	}
	return storageError(message, err)
}

func storageError(message string, err error) error {
	return NewError(ErrorKindStorage, message, err)
}
