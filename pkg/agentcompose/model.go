package agentcompose

import (
	sessionmodel "agent-compose/pkg/agentcompose/session"
	"time"
)

const (
	VMStatusPending = sessionmodel.VMStatusPending
	VMStatusRunning = sessionmodel.VMStatusRunning
	VMStatusStopped = sessionmodel.VMStatusStopped
	VMStatusFailed  = sessionmodel.VMStatusFailed

	SessionTypeManual = sessionmodel.SessionTypeManual
	SessionTypeScript = sessionmodel.SessionTypeScript
)

type SessionTag = sessionmodel.SessionTag
type SessionEnvVar = sessionmodel.SessionEnvVar
type SessionSummary = sessionmodel.SessionSummary
type SessionListOptions = sessionmodel.SessionListOptions
type SessionListResult = sessionmodel.SessionListResult
type SessionWorkspace = sessionmodel.SessionWorkspace
type Session = sessionmodel.Session
type NotebookCell = sessionmodel.NotebookCell
type AgentResumeInfo = sessionmodel.AgentResumeInfo
type ExecChunk = sessionmodel.ExecChunk
type SessionEvent = sessionmodel.SessionEvent
type AgentRun = sessionmodel.AgentRun
type ExecResult = sessionmodel.ExecResult
type RuntimeCommandArtifacts = sessionmodel.RuntimeCommandArtifacts
type RuntimeCommandResult = sessionmodel.RuntimeCommandResult
type ExecStreamWriter = sessionmodel.ExecStreamWriter
type VMState = sessionmodel.VMState
type ProxyState = sessionmodel.ProxyState
type ExecSpec = sessionmodel.ExecSpec
type AgentRunResult = sessionmodel.AgentRunResult

type WorkspaceConfig struct {
	ID         string    `json:"id"`
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	ConfigJSON string    `json:"config_json"`
	Comment    string    `json:"comment,omitempty"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

func sessionEnvMap(groups ...[]SessionEnvVar) map[string]string {
	return sessionmodel.EnvMap(groups...)
}

func restoreSessionTransientFields(dst, src *Session) {
	sessionmodel.RestoreTransientFields(dst, src)
}
