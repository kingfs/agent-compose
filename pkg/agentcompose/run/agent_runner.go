package run

import (
	"context"

	"agent-compose/pkg/agentcompose/project"
	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

type AgentStream interface{}

type ProjectAgentRunner interface {
	RunProjectAgent(ctx context.Context, msg *agentcomposev2.RunAgentRequest, stream AgentStream) (project.RunRecord, error, error)
}
