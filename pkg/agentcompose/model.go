package agentcompose

import (
	"agent-compose/pkg/agentcompose/configsvc"
	sessionmodel "agent-compose/pkg/agentcompose/session"
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

type WorkspaceConfig = configsvc.WorkspaceConfig

func sessionEnvMap(groups ...[]SessionEnvVar) map[string]string {
	return sessionmodel.EnvMap(groups...)
}

func restoreSessionTransientFields(dst, src *Session) {
	sessionmodel.RestoreTransientFields(dst, src)
}
