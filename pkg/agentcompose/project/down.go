package project

import (
	"context"
	"fmt"
	"strings"

	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

type DownStore interface {
	ListProjectSchedulers(ctx context.Context, projectID string) ([]SchedulerRecord, error)
	SetProjectSchedulerEnabled(ctx context.Context, projectID, schedulerID string, enabled bool) (SchedulerRecord, error)
}

type LoaderController interface {
	DisableManagedLoaderIfOwned(ctx context.Context, loaderID, projectID, schedulerID string) error
	RefreshLoaders(ctx context.Context) error
}

type SessionController interface {
	ListRunningSessions(ctx context.Context) ([]SessionSummary, error)
	StopProjectRunSession(ctx context.Context, sessionID string) error
}

type SessionSummary struct {
	ID    string
	Title string
	Tags  []SessionTag
}

type SessionTag struct {
	Name  string
	Value string
}

type DownService struct {
	Store    DownStore
	Loaders  LoaderController
	Sessions SessionController
}

func (s DownService) DownProject(ctx context.Context, project ProjectRecord) ([]*agentcomposev2.ProjectChange, error) {
	var changes []*agentcomposev2.ProjectChange
	schedulerChanges, err := s.DisableManagedSchedulers(ctx, project)
	if err != nil {
		return changes, err
	}
	changes = append(changes, schedulerChanges...)
	sessionChanges, err := s.StopRunningSessions(ctx, project)
	if err != nil {
		return changes, err
	}
	changes = append(changes, sessionChanges...)
	return changes, nil
}

func (s DownService) DisableManagedSchedulers(ctx context.Context, project ProjectRecord) ([]*agentcomposev2.ProjectChange, error) {
	if s.Store == nil {
		return nil, fmt.Errorf("config store is required")
	}
	schedulers, err := s.Store.ListProjectSchedulers(ctx, project.ID)
	if err != nil {
		return nil, fmt.Errorf("list project schedulers for down %s: %w", project.Name, err)
	}
	var changes []*agentcomposev2.ProjectChange
	for _, scheduler := range schedulers {
		if !scheduler.Enabled {
			continue
		}
		disabled, err := s.Store.SetProjectSchedulerEnabled(ctx, scheduler.ProjectID, scheduler.SchedulerID, false)
		if err != nil {
			return changes, fmt.Errorf("disable project scheduler %s/%s: %w", scheduler.ProjectID, scheduler.SchedulerID, err)
		}
		if s.Loaders != nil {
			if err := s.Loaders.DisableManagedLoaderIfOwned(ctx, scheduler.ManagedLoaderID, project.ID, scheduler.SchedulerID); err != nil {
				return changes, fmt.Errorf("disable managed loader %s: %w", scheduler.ManagedLoaderID, err)
			}
		}
		changes = append(changes, &agentcomposev2.ProjectChange{Action: agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_UPDATED, ResourceType: "project_scheduler", ResourceId: disabled.SchedulerID, Name: disabled.AgentName, Message: "disabled by project down"})
		if scheduler.ManagedLoaderID != "" {
			changes = append(changes, &agentcomposev2.ProjectChange{Action: agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_UPDATED, ResourceType: "loader", ResourceId: scheduler.ManagedLoaderID, Name: scheduler.AgentName, Message: "disabled by project down"})
		}
	}
	if len(changes) > 0 && s.Loaders != nil {
		if err := s.Loaders.RefreshLoaders(ctx); err != nil {
			return changes, fmt.Errorf("refresh loader manager after project down: %w", err)
		}
	}
	return changes, nil
}

func (s DownService) StopRunningSessions(ctx context.Context, project ProjectRecord) ([]*agentcomposev2.ProjectChange, error) {
	if s.Sessions == nil {
		return nil, fmt.Errorf("session store is required")
	}
	sessions, err := s.Sessions.ListRunningSessions(ctx)
	if err != nil {
		return nil, fmt.Errorf("list running sessions for project down %s: %w", project.Name, err)
	}
	var changes []*agentcomposev2.ProjectChange
	for _, session := range sessions {
		if !SessionHasTag(session, "project", project.ID) {
			continue
		}
		if err := s.Sessions.StopProjectRunSession(ctx, session.ID); err != nil {
			changes = append(changes, &agentcomposev2.ProjectChange{Action: agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_UNCHANGED, ResourceType: "session", ResourceId: session.ID, Name: session.Title, Message: fmt.Sprintf("failed to stop by project down: %v", err)})
			continue
		}
		changes = append(changes, &agentcomposev2.ProjectChange{Action: agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_UPDATED, ResourceType: "session", ResourceId: session.ID, Name: session.Title, Message: "stopped by project down"})
	}
	return changes, nil
}

func SessionHasTag(session SessionSummary, name, value string) bool {
	name = strings.TrimSpace(name)
	value = strings.TrimSpace(value)
	for _, tag := range session.Tags {
		if strings.TrimSpace(tag.Name) == name && strings.TrimSpace(tag.Value) == value {
			return true
		}
	}
	return false
}
