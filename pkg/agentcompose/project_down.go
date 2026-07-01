package agentcompose

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	projectpkg "agent-compose/pkg/agentcompose/project"
	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

func (s *Service) downProject(ctx context.Context, project ProjectRecord) ([]*agentcomposev2.ProjectChange, error) {
	changes, err := projectpkg.DownService{Store: projectDownStore{store: s.configDB}, Loaders: projectDownLoaders{service: s}, Sessions: projectDownSessions{service: s}}.DownProject(ctx, projectpkg.ProjectRecord(project))
	if err != nil {
		return changes, connect.NewError(connect.CodeInternal, err)
	}
	return changes, nil
}

func (s *Service) disableProjectManagedSchedulers(ctx context.Context, project ProjectRecord) ([]*agentcomposev2.ProjectChange, error) {
	return projectpkg.DownService{Store: projectDownStore{store: s.configDB}, Loaders: projectDownLoaders{service: s}}.DisableManagedSchedulers(ctx, projectpkg.ProjectRecord(project))
}

func (s *Service) stopProjectRunningSessions(ctx context.Context, project ProjectRecord) ([]*agentcomposev2.ProjectChange, error) {
	return projectpkg.DownService{Sessions: projectDownSessions{service: s}}.StopRunningSessions(ctx, projectpkg.ProjectRecord(project))
}

type projectDownStore struct {
	store *ConfigStore
}

func (s projectDownStore) ListProjectSchedulers(ctx context.Context, projectID string) ([]projectpkg.SchedulerRecord, error) {
	items, err := s.store.ListProjectSchedulers(ctx, projectID)
	converted := make([]projectpkg.SchedulerRecord, 0, len(items))
	for _, item := range items {
		converted = append(converted, projectpkg.SchedulerRecord(item))
	}
	return converted, err
}

func (s projectDownStore) SetProjectSchedulerEnabled(ctx context.Context, projectID, schedulerID string, enabled bool) (projectpkg.SchedulerRecord, error) {
	record, err := s.store.SetProjectSchedulerEnabled(ctx, projectID, schedulerID, enabled)
	return projectpkg.SchedulerRecord(record), err
}

type projectDownLoaders struct {
	service *Service
}

func (l projectDownLoaders) DisableManagedLoaderIfOwned(ctx context.Context, loaderID, projectID, schedulerID string) error {
	return l.service.disableManagedLoaderIfOwned(ctx, loaderID, projectID, schedulerID)
}

func (l projectDownLoaders) RefreshLoaders(ctx context.Context) error {
	if l.service.loaders == nil {
		return nil
	}
	return l.service.loaders.Refresh(ctx)
}

type projectDownSessions struct {
	service *Service
}

func (s projectDownSessions) ListRunningSessions(ctx context.Context) ([]projectpkg.SessionSummary, error) {
	if s.service.store == nil {
		return nil, fmt.Errorf("session store is required")
	}
	result, err := s.service.store.ListSessions(ctx, SessionListOptions{VMStatus: VMStatusRunning, Limit: 1 << 30})
	if err != nil {
		return nil, err
	}
	items := make([]projectpkg.SessionSummary, 0, len(result.Sessions))
	for _, session := range result.Sessions {
		items = append(items, projectSessionSummaryForDown(session))
	}
	return items, nil
}

func (s projectDownSessions) StopProjectRunSession(ctx context.Context, sessionID string) error {
	session, err := s.service.store.GetSession(ctx, sessionID)
	if err != nil {
		return err
	}
	return s.service.stopProjectRunSession(ctx, session)
}

func projectSessionSummaryForDown(session *Session) projectpkg.SessionSummary {
	if session == nil {
		return projectpkg.SessionSummary{}
	}
	tags := make([]projectpkg.SessionTag, 0, len(session.Summary.Tags))
	for _, tag := range session.Summary.Tags {
		tags = append(tags, projectpkg.SessionTag{Name: tag.Name, Value: tag.Value})
	}
	return projectpkg.SessionSummary{ID: session.Summary.ID, Title: session.Summary.Title, Tags: tags}
}

func projectSessionHasTag(session *Session, name, value string) bool {
	if session == nil {
		return false
	}
	return projectpkg.SessionHasTag(projectSessionSummaryForDown(session), strings.TrimSpace(name), strings.TrimSpace(value))
}
