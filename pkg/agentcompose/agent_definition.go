package agentcompose

import (
	agentdefpkg "agent-compose/pkg/agentcompose/agentdef"
	agentcomposev1 "agent-compose/proto/agentcompose/v1"
)

const (
	defaultAgentProvider = "codex"

	agentSessionTagSource    = "source"
	agentSessionTagSourceVal = "agent"
	agentSessionTagID        = "agent_id"
	agentSessionTagName      = "agent_name"
)

type AgentDefinition = agentdefpkg.AgentDefinition
type AgentDefinitionListOptions = agentdefpkg.AgentDefinitionListOptions
type AgentDefinitionListResult = agentdefpkg.AgentDefinitionListResult
type AgentValidationResult = agentdefpkg.AgentValidationResult
type AgentCurrentRunSummary = agentdefpkg.AgentCurrentRunSummary
type AgentLatestRunSummary = agentdefpkg.AgentLatestRunSummary

func normalizeAgentDefinition(item AgentDefinition, assignDefaults bool) (AgentDefinition, error) {
	return agentdefpkg.NormalizeAgentDefinition(item, assignDefaults)
}

func agentDefinitionTags(agent AgentDefinition) []*agentcomposev1.SessionTag {
	return agentdefpkg.AgentDefinitionTags(agent)
}

func sessionHasAgentTag(session *Session, agentID string) bool {
	return agentdefpkg.SessionHasAgentTag(agentdefSession(session), agentID)
}

func toProtoAgentDefinition(item AgentDefinition, workspace *WorkspaceConfig, validation AgentValidationResult, current AgentCurrentRunSummary, latest *AgentLatestRunSummary) *agentcomposev1.AgentDefinition {
	return agentdefpkg.ToProtoAgentDefinition(item, workspace, validation, current, latest)
}

func toProtoEnvItems(items []SessionEnvVar) []*agentcomposev1.SessionEnvVar {
	return agentdefpkg.ToProtoEnvItems(items)
}

func agentdefSession(session *Session) *agentdefpkg.Session {
	if session == nil {
		return nil
	}
	tags := make([]agentdefpkg.SessionTag, 0, len(session.Summary.Tags))
	for _, tag := range session.Summary.Tags {
		tags = append(tags, agentdefpkg.SessionTag{Name: tag.Name, Value: tag.Value})
	}
	return &agentdefpkg.Session{Summary: agentdefpkg.SessionSummary{Tags: tags}}
}
