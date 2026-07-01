package agentcompose

import (
	"context"
	"fmt"

	agentcomposeimage "agent-compose/pkg/agentcompose/image"
)

type driverImageEnsureRequest struct {
	Driver      string
	ImageRef    string
	ProjectName string
	AgentName   string
}

func (s *Service) ensureProjectAgentImages(ctx context.Context, projectName string, agents []ProjectAgentRecord) error {
	if s == nil || s.config == nil {
		return fmt.Errorf("image ensure config is required")
	}
	imageAgents := make([]agentcomposeimage.ProjectAgent, 0, len(agents))
	for _, agent := range agents {
		imageAgents = append(imageAgents, agentcomposeimage.ProjectAgent{
			Driver:    agent.Driver,
			Image:     agent.Image,
			AgentName: agent.AgentName,
		})
	}
	return agentcomposeimage.EnsureProjectAgentImages(ctx, s.config, s.images, projectName, imageAgents)
}

func (s *Service) ensureDriverImage(ctx context.Context, req driverImageEnsureRequest) error {
	if s == nil || s.config == nil {
		return fmt.Errorf("image ensure config is required")
	}
	return agentcomposeimage.EnsureDriverImage(ctx, s.images, agentcomposeimage.DriverEnsureRequest{
		Driver:      req.Driver,
		ImageRef:    req.ImageRef,
		ProjectName: req.ProjectName,
		AgentName:   req.AgentName,
	})
}

func imageBackendErrorIsNotFound(err error) bool {
	return agentcomposeimage.BackendErrorIsNotFound(err)
}
