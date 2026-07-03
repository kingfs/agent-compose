package projecttypes

import "time"

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

type ProjectRevisionRecord struct {
	ProjectID string    `json:"project_id"`
	Revision  int64     `json:"revision"`
	SpecHash  string    `json:"spec_hash"`
	SpecJSON  string    `json:"spec_json"`
	CreatedAt time.Time `json:"created_at"`
}

type ProjectAgentRecord struct {
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

type ProjectSchedulerRecord struct {
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

type ProjectListOptions struct {
	Query          string
	IncludeRemoved bool
	Offset         int
	Limit          int
}

type ProjectListResult struct {
	Projects   []ProjectRecord
	TotalCount int
	HasMore    bool
	NextOffset int
}
