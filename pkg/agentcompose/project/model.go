package project

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"agent-compose/pkg/compose"
)

const (
	RunStatusPending   = "pending"
	RunStatusRunning   = "running"
	RunStatusSucceeded = "succeeded"
	RunStatusFailed    = "failed"
	RunStatusCanceled  = "canceled"

	RunSourceManual    = "manual"
	RunSourceScheduler = "scheduler"
	RunSourceAPI       = "api"
)

type ProjectRecord struct {
	ID              string    `json:"id"`
	Name            string    `json:"name"`
	SourcePath      string    `json:"source_path,omitempty"`
	SourceJSON      string    `json:"source_json"`
	CurrentRevision int64     `json:"current_revision"`
	SpecHash        string    `json:"spec_hash,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
	RemovedAt       time.Time `json:"removed_at,omitempty"`
}

type RevisionRecord struct {
	ProjectID string    `json:"project_id"`
	Revision  int64     `json:"revision"`
	SpecHash  string    `json:"spec_hash"`
	SpecJSON  string    `json:"spec_json"`
	CreatedAt time.Time `json:"created_at"`
}

type AgentRecord struct {
	ProjectID        string    `json:"project_id"`
	AgentName        string    `json:"agent_name"`
	ManagedAgentID   string    `json:"managed_agent_id,omitempty"`
	Revision         int64     `json:"revision"`
	Provider         string    `json:"provider,omitempty"`
	Model            string    `json:"model,omitempty"`
	Image            string    `json:"image,omitempty"`
	Driver           string    `json:"driver,omitempty"`
	SchedulerEnabled bool      `json:"scheduler_enabled"`
	SpecJSON         string    `json:"spec_json"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
}

type SchedulerRecord struct {
	ProjectID       string    `json:"project_id"`
	SchedulerID     string    `json:"scheduler_id"`
	AgentName       string    `json:"agent_name"`
	ManagedLoaderID string    `json:"managed_loader_id,omitempty"`
	Revision        int64     `json:"revision"`
	Enabled         bool      `json:"enabled"`
	TriggerCount    int       `json:"trigger_count"`
	SpecJSON        string    `json:"spec_json"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type RunRecord struct {
	RunID           string    `json:"run_id"`
	ProjectID       string    `json:"project_id"`
	ProjectName     string    `json:"project_name,omitempty"`
	ProjectRevision int64     `json:"project_revision"`
	AgentName       string    `json:"agent_name,omitempty"`
	ManagedAgentID  string    `json:"managed_agent_id,omitempty"`
	Source          string    `json:"source,omitempty"`
	SchedulerID     string    `json:"scheduler_id,omitempty"`
	TriggerID       string    `json:"trigger_id,omitempty"`
	Status          string    `json:"status"`
	SessionID       string    `json:"session_id,omitempty"`
	ExitCode        int       `json:"exit_code,omitempty"`
	Error           string    `json:"error,omitempty"`
	Prompt          string    `json:"prompt,omitempty"`
	Output          string    `json:"output,omitempty"`
	ResultJSON      string    `json:"result_json,omitempty"`
	LogsPath        string    `json:"logs_path,omitempty"`
	ArtifactsDir    string    `json:"artifacts_dir,omitempty"`
	CleanupError    string    `json:"cleanup_error,omitempty"`
	Driver          string    `json:"driver,omitempty"`
	ImageRef        string    `json:"image_ref,omitempty"`
	StartedAt       time.Time `json:"started_at,omitempty"`
	CompletedAt     time.Time `json:"completed_at,omitempty"`
	DurationMs      int64     `json:"duration_ms,omitempty"`
	CreatedAt       time.Time `json:"created_at"`
	UpdatedAt       time.Time `json:"updated_at"`
}

type ListOptions struct {
	Query          string
	IncludeRemoved bool
	Offset         int
	Limit          int
}

type RunListOptions struct {
	ProjectID   string
	AgentName   string
	SessionID   string
	SchedulerID string
	Status      string
	Source      string
	Offset      int
	Limit       int
}

type ListResult struct {
	Projects   []ProjectRecord
	TotalCount int
	HasMore    bool
	NextOffset int
}

func StableProjectID(name, sourcePath string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", fmt.Errorf("project name is required")
	}
	if !IsStableIdentifier(name) {
		return "", fmt.Errorf("project name %q is not a stable identifier", name)
	}
	return stableReadableID("project", name, name+"|"+NormalizeSourcePath(sourcePath)), nil
}

func StableManagedAgentID(projectID, agentName string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	agentName = strings.TrimSpace(agentName)
	if projectID == "" || agentName == "" {
		return "", fmt.Errorf("project id and agent name are required")
	}
	if !IsStableIdentifier(agentName) {
		return "", fmt.Errorf("agent name %q is not a stable identifier", agentName)
	}
	return stableReadableID("agent", agentName, projectID+"|"+agentName), nil
}

func StableSchedulerID(projectID, agentName, schedulerName string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	agentName = strings.TrimSpace(agentName)
	schedulerName = strings.TrimSpace(schedulerName)
	if schedulerName == "" {
		schedulerName = "default"
	}
	if projectID == "" || agentName == "" {
		return "", fmt.Errorf("project id and agent name are required")
	}
	if !IsStableIdentifier(agentName) {
		return "", fmt.Errorf("agent name %q is not a stable identifier", agentName)
	}
	if !IsStableIdentifier(schedulerName) {
		return "", fmt.Errorf("scheduler name %q is not a stable identifier", schedulerName)
	}
	return stableReadableID("scheduler", agentName+"-"+schedulerName, projectID+"|"+agentName+"|"+schedulerName), nil
}

func StableManagedLoaderID(projectID, agentName, schedulerName string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	agentName = strings.TrimSpace(agentName)
	schedulerName = strings.TrimSpace(schedulerName)
	if schedulerName == "" {
		schedulerName = "default"
	}
	if projectID == "" || agentName == "" {
		return "", fmt.Errorf("project id and agent name are required")
	}
	if !IsStableIdentifier(agentName) {
		return "", fmt.Errorf("agent name %q is not a stable identifier", agentName)
	}
	if !IsStableIdentifier(schedulerName) {
		return "", fmt.Errorf("scheduler name %q is not a stable identifier", schedulerName)
	}
	return stableReadableID("loader", agentName+"-"+schedulerName, projectID+"|"+agentName+"|"+schedulerName), nil
}

func StableManagedTriggerID(projectID, agentName, schedulerName, triggerName string, triggerIndex int) (string, error) {
	projectID = strings.TrimSpace(projectID)
	agentName = strings.TrimSpace(agentName)
	schedulerName = strings.TrimSpace(schedulerName)
	triggerName = strings.TrimSpace(triggerName)
	if schedulerName == "" {
		schedulerName = "default"
	}
	if projectID == "" || agentName == "" || triggerIndex < 0 {
		return "", fmt.Errorf("project id, agent name, and trigger index are required")
	}
	if !IsStableIdentifier(agentName) {
		return "", fmt.Errorf("agent name %q is not a stable identifier", agentName)
	}
	if !IsStableIdentifier(schedulerName) {
		return "", fmt.Errorf("scheduler name %q is not a stable identifier", schedulerName)
	}
	readable := triggerName
	seedPart := "name:" + triggerName
	if readable == "" {
		readable = fmt.Sprintf("trigger-%d", triggerIndex+1)
		seedPart = fmt.Sprintf("path:triggers[%d]", triggerIndex)
	}
	return stableReadableID("trigger", readable, projectID+"|"+agentName+"|"+schedulerName+"|"+seedPart), nil
}

func StableRunID(projectID, agentName, source, idempotencyKey string) (string, error) {
	projectID = strings.TrimSpace(projectID)
	agentName = strings.TrimSpace(agentName)
	source = strings.TrimSpace(source)
	idempotencyKey = strings.TrimSpace(idempotencyKey)
	if projectID == "" || agentName == "" || source == "" || idempotencyKey == "" {
		return "", fmt.Errorf("project id, agent name, source, and idempotency key are required")
	}
	if !IsStableIdentifier(agentName) {
		return "", fmt.Errorf("agent name %q is not a stable identifier", agentName)
	}
	return stableReadableID("run", agentName, projectID+"|"+agentName+"|"+source+"|"+idempotencyKey), nil
}

func NewRecordFromSpec(spec *compose.NormalizedProjectSpec, sourcePath string) (ProjectRecord, error) {
	if spec == nil {
		return ProjectRecord{}, fmt.Errorf("project spec is required")
	}
	sourcePath = NormalizeSourcePath(sourcePath)
	projectID, err := StableProjectID(spec.Name, sourcePath)
	if err != nil {
		return ProjectRecord{}, err
	}
	specHash, err := spec.Hash()
	if err != nil {
		return ProjectRecord{}, fmt.Errorf("hash project spec: %w", err)
	}
	sourceJSON, err := EncodeSourceJSON(sourcePath)
	if err != nil {
		return ProjectRecord{}, err
	}
	return ProjectRecord{ID: projectID, Name: strings.TrimSpace(spec.Name), SourcePath: sourcePath, SourceJSON: sourceJSON, SpecHash: specHash}, nil
}

func NewAgentRecordFromSpec(projectID string, revision int64, agent compose.NormalizedAgentSpec) (AgentRecord, error) {
	managedAgentID, err := StableManagedAgentID(projectID, agent.Name)
	if err != nil {
		return AgentRecord{}, err
	}
	specJSON, err := MarshalCanonicalJSON(agent)
	if err != nil {
		return AgentRecord{}, fmt.Errorf("marshal project agent %s spec: %w", agent.Name, err)
	}
	driver := ""
	if agent.Driver != nil {
		driver = agent.Driver.Name
	}
	return AgentRecord{ProjectID: strings.TrimSpace(projectID), AgentName: strings.TrimSpace(agent.Name), ManagedAgentID: managedAgentID, Revision: revision, Provider: strings.TrimSpace(agent.Provider), Model: strings.TrimSpace(agent.Model), Image: strings.TrimSpace(agent.Image), Driver: strings.TrimSpace(driver), SchedulerEnabled: agent.Scheduler != nil && agent.Scheduler.Enabled, SpecJSON: string(specJSON)}, nil
}

func NewSchedulerRecordFromSpec(projectID string, revision int64, agent compose.NormalizedAgentSpec) (SchedulerRecord, bool, error) {
	if agent.Scheduler == nil {
		return SchedulerRecord{}, false, nil
	}
	schedulerID, err := StableSchedulerID(projectID, agent.Name, "")
	if err != nil {
		return SchedulerRecord{}, false, err
	}
	loaderID, err := StableManagedLoaderID(projectID, agent.Name, "")
	if err != nil {
		return SchedulerRecord{}, false, err
	}
	specJSON, err := MarshalCanonicalJSON(agent.Scheduler)
	if err != nil {
		return SchedulerRecord{}, false, fmt.Errorf("marshal project scheduler %s spec: %w", agent.Name, err)
	}
	return SchedulerRecord{ProjectID: strings.TrimSpace(projectID), SchedulerID: schedulerID, AgentName: strings.TrimSpace(agent.Name), ManagedLoaderID: loaderID, Revision: revision, Enabled: agent.Scheduler.Enabled, TriggerCount: len(agent.Scheduler.Triggers), SpecJSON: string(specJSON)}, true, nil
}

func NormalizeRunStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case RunStatusPending:
		return RunStatusPending
	case RunStatusRunning:
		return RunStatusRunning
	case RunStatusSucceeded:
		return RunStatusSucceeded
	case RunStatusFailed:
		return RunStatusFailed
	case RunStatusCanceled:
		return RunStatusCanceled
	default:
		return RunStatusPending
	}
}

func NormalizeRunSource(source string) string {
	switch strings.ToLower(strings.TrimSpace(source)) {
	case RunSourceScheduler:
		return RunSourceScheduler
	case RunSourceAPI:
		return RunSourceAPI
	case RunSourceManual:
		return RunSourceManual
	default:
		return RunSourceManual
	}
}

func NormalizeSourcePath(sourcePath string) string {
	sourcePath = strings.TrimSpace(sourcePath)
	if sourcePath == "" {
		return ""
	}
	if abs, err := filepath.Abs(sourcePath); err == nil {
		sourcePath = abs
	}
	return filepath.Clean(sourcePath)
}

func EncodeSourceJSON(sourcePath string) (string, error) {
	data, err := json.Marshal(struct {
		ComposePath string `json:"compose_path,omitempty"`
	}{ComposePath: NormalizeSourcePath(sourcePath)})
	if err != nil {
		return "", fmt.Errorf("marshal project source: %w", err)
	}
	return string(data), nil
}

func MarshalCanonicalJSON(value any) ([]byte, error) {
	return json.Marshal(value)
}

func IsStableIdentifier(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	for i, r := range value {
		switch {
		case i == 0 && r >= 'a' && r <= 'z':
		case i > 0 && r >= 'a' && r <= 'z':
		case i > 0 && r >= '0' && r <= '9':
		case i > 0 && (r == '-' || r == '_'):
		default:
			return false
		}
	}
	return true
}

func StableReadableIDForWorkspace(readable, seed string) string {
	return stableReadableID("workspace", readable, seed)
}

func stableReadableID(prefix, readable, seed string) string {
	readable = strings.ToLower(strings.TrimSpace(readable))
	var b strings.Builder
	for _, r := range readable {
		switch {
		case r >= 'a' && r <= 'z':
			b.WriteRune(r)
		case r >= '0' && r <= '9':
			b.WriteRune(r)
		case r == '-' || r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	readable = strings.Trim(b.String(), "-_")
	if readable == "" {
		readable = "item"
	}
	if len(readable) > 48 {
		readable = strings.Trim(readable[:48], "-_")
	}
	sum := sha256.Sum256([]byte(seed))
	return prefix + "-" + readable + "-" + hex.EncodeToString(sum[:6])
}
