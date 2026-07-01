package run

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"agent-compose/pkg/agentcompose/project"
)

type StartRequest struct {
	ProjectID       string
	AgentName       string
	Source          string
	SchedulerID     string
	TriggerID       string
	Prompt          string
	ClientRequestID string
}

type TransitionRequest struct {
	RunID        string
	Status       string
	SessionID    string
	ExitCode     int
	Error        string
	Output       string
	ResultJSON   string
	LogsPath     string
	ArtifactsDir string
	CleanupError string
}

type ManagedAgentDefinition struct {
	ID                     string
	Enabled                bool
	DeletedAt              time.Time
	Driver                 string
	GuestImage             string
	ManagedProjectID       string
	ManagedProjectRevision int64
	ManagedAgentName       string
}

type Store interface {
	GetProject(ctx context.Context, projectID string) (project.ProjectRecord, error)
	GetProjectAgent(ctx context.Context, projectID, agentName string) (project.AgentRecord, error)
	GetManagedAgentDefinition(ctx context.Context, agentID string) (ManagedAgentDefinition, error)
	CreateProjectRun(ctx context.Context, record project.RunRecord) (project.RunRecord, error)
	UpdateProjectRun(ctx context.Context, record project.RunRecord) (project.RunRecord, error)
	GetProjectRun(ctx context.Context, runID string) (project.RunRecord, error)
}

type Coordinator struct {
	store Store
	now   func() time.Time
}

func NewCoordinator(store Store) *Coordinator {
	return &Coordinator{store: store, now: func() time.Time { return time.Now().UTC() }}
}

func (c *Coordinator) SetNow(now func() time.Time) {
	c.now = now
}

func (c *Coordinator) BeginRun(ctx context.Context, req StartRequest) (project.RunRecord, error) {
	if c == nil || c.store == nil {
		return project.RunRecord{}, fmt.Errorf("config store is required")
	}
	req.ProjectID = strings.TrimSpace(req.ProjectID)
	req.AgentName = strings.TrimSpace(req.AgentName)
	req.Source = project.NormalizeRunSource(req.Source)
	req.SchedulerID = strings.TrimSpace(req.SchedulerID)
	req.TriggerID = strings.TrimSpace(req.TriggerID)
	req.ClientRequestID = strings.TrimSpace(req.ClientRequestID)
	if req.ProjectID == "" || req.AgentName == "" {
		return project.RunRecord{}, fmt.Errorf("project id and agent name are required")
	}
	if req.ClientRequestID == "" {
		req.ClientRequestID = uuid.NewString()
	}
	projectRecord, err := c.store.GetProject(ctx, req.ProjectID)
	if err != nil {
		return project.RunRecord{}, fmt.Errorf("resolve project %s: %w", req.ProjectID, err)
	}
	projectAgent, err := c.store.GetProjectAgent(ctx, projectRecord.ID, req.AgentName)
	if err != nil {
		return project.RunRecord{}, fmt.Errorf("resolve project agent %s/%s: %w", projectRecord.ID, req.AgentName, err)
	}
	agent, err := c.store.GetManagedAgentDefinition(ctx, projectAgent.ManagedAgentID)
	if err != nil {
		return project.RunRecord{}, fmt.Errorf("resolve managed agent definition %s: %w", projectAgent.ManagedAgentID, err)
	}
	if !agent.Enabled || !agent.DeletedAt.IsZero() {
		return project.RunRecord{}, fmt.Errorf("managed agent definition %s is disabled", agent.ID)
	}
	if agent.ManagedProjectID != projectRecord.ID || agent.ManagedAgentName != projectAgent.AgentName {
		return project.RunRecord{}, fmt.Errorf("managed agent definition %s does not belong to project agent %s/%s", agent.ID, projectRecord.ID, projectAgent.AgentName)
	}
	runID, err := project.StableRunID(projectRecord.ID, projectAgent.AgentName, req.Source, req.ClientRequestID)
	if err != nil {
		return project.RunRecord{}, err
	}
	record := project.RunRecord{
		RunID:           runID,
		ProjectID:       projectRecord.ID,
		ProjectName:     projectRecord.Name,
		ProjectRevision: projectRecord.CurrentRevision,
		AgentName:       projectAgent.AgentName,
		ManagedAgentID:  agent.ID,
		Source:          req.Source,
		SchedulerID:     req.SchedulerID,
		TriggerID:       req.TriggerID,
		Status:          project.RunStatusPending,
		Prompt:          req.Prompt,
		Driver:          firstNonEmpty(agent.Driver, projectAgent.Driver),
		ImageRef:        firstNonEmpty(agent.GuestImage, projectAgent.Image),
		ResultJSON:      "{}",
	}
	created, err := c.store.CreateProjectRun(ctx, record)
	if err == nil {
		return created, nil
	}
	if existing, loadErr := c.store.GetProjectRun(ctx, runID); loadErr == nil {
		return existing, nil
	}
	return project.RunRecord{}, err
}

func (c *Coordinator) MarkRunning(ctx context.Context, runID, sessionID string) (project.RunRecord, error) {
	return c.TransitionRun(ctx, TransitionRequest{RunID: runID, Status: project.RunStatusRunning, SessionID: sessionID})
}

func (c *Coordinator) MarkSucceeded(ctx context.Context, req TransitionRequest) (project.RunRecord, error) {
	req.Status = project.RunStatusSucceeded
	return c.TransitionRun(ctx, req)
}

func (c *Coordinator) MarkFailed(ctx context.Context, req TransitionRequest) (project.RunRecord, error) {
	req.Status = project.RunStatusFailed
	return c.TransitionRun(ctx, req)
}

func (c *Coordinator) MarkCanceled(ctx context.Context, req TransitionRequest) (project.RunRecord, error) {
	req.Status = project.RunStatusCanceled
	return c.TransitionRun(ctx, req)
}

func (c *Coordinator) TransitionRun(ctx context.Context, req TransitionRequest) (project.RunRecord, error) {
	if c == nil || c.store == nil {
		return project.RunRecord{}, fmt.Errorf("config store is required")
	}
	req.RunID = strings.TrimSpace(req.RunID)
	req.Status = project.NormalizeRunStatus(req.Status)
	if req.RunID == "" {
		return project.RunRecord{}, fmt.Errorf("run id is required")
	}
	current, err := c.store.GetProjectRun(ctx, req.RunID)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return project.RunRecord{}, err
		}
		return project.RunRecord{}, err
	}
	if err := ValidateTransition(current.Status, req.Status); err != nil {
		return project.RunRecord{}, err
	}
	now := c.nowUTC()
	next := current
	next.Status = req.Status
	ApplyTransitionFields(&next, req)
	switch req.Status {
	case project.RunStatusRunning:
		if next.StartedAt.IsZero() {
			next.StartedAt = now
		}
	case project.RunStatusSucceeded, project.RunStatusFailed, project.RunStatusCanceled:
		if next.StartedAt.IsZero() {
			next.StartedAt = now
		}
		if next.CompletedAt.IsZero() {
			next.CompletedAt = now
		}
		next.DurationMs = max(0, next.CompletedAt.Sub(next.StartedAt).Milliseconds())
	}
	return c.store.UpdateProjectRun(ctx, next)
}

func (c *Coordinator) nowUTC() time.Time {
	if c != nil && c.now != nil {
		return c.now().UTC()
	}
	return time.Now().UTC()
}

func ApplyTransitionFields(record *project.RunRecord, req TransitionRequest) {
	if value := strings.TrimSpace(req.SessionID); value != "" {
		record.SessionID = value
	}
	if req.ExitCode != 0 {
		record.ExitCode = req.ExitCode
	}
	if value := strings.TrimSpace(req.Error); value != "" {
		record.Error = value
	}
	if req.Output != "" {
		record.Output = req.Output
	}
	if value := strings.TrimSpace(req.ResultJSON); value != "" {
		record.ResultJSON = value
	}
	if value := strings.TrimSpace(req.LogsPath); value != "" {
		record.LogsPath = value
	}
	if value := strings.TrimSpace(req.ArtifactsDir); value != "" {
		record.ArtifactsDir = value
	}
	if value := strings.TrimSpace(req.CleanupError); value != "" {
		record.CleanupError = value
	}
}

func ValidateTransition(from, to string) error {
	from = project.NormalizeRunStatus(from)
	to = project.NormalizeRunStatus(to)
	if from == to {
		return nil
	}
	if StatusIsTerminal(from) {
		return fmt.Errorf("project run transition %s -> %s is not allowed: run is already terminal", from, to)
	}
	switch from {
	case project.RunStatusPending:
		switch to {
		case project.RunStatusRunning, project.RunStatusFailed, project.RunStatusCanceled:
			return nil
		}
	case project.RunStatusRunning:
		switch to {
		case project.RunStatusSucceeded, project.RunStatusFailed, project.RunStatusCanceled:
			return nil
		}
	}
	return fmt.Errorf("project run transition %s -> %s is not allowed", from, to)
}

func StatusIsTerminal(status string) bool {
	switch project.NormalizeRunStatus(status) {
	case project.RunStatusSucceeded, project.RunStatusFailed, project.RunStatusCanceled:
		return true
	default:
		return false
	}
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if strings.TrimSpace(value) != "" {
			return strings.TrimSpace(value)
		}
	}
	return ""
}
