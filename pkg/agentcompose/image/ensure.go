package image

import (
	"context"
	"errors"
	"fmt"
	"strings"

	cerrdefs "github.com/containerd/errdefs"

	appconfig "agent-compose/pkg/config"
	driverpkg "agent-compose/pkg/driver"
)

type ProjectAgent struct {
	Driver    string
	Image     string
	AgentName string
}

type DriverEnsureRequest struct {
	Driver      string
	ImageRef    string
	ProjectName string
	AgentName   string
}

func EnsureProjectAgentImages(ctx context.Context, config *appconfig.Config, backend ImageBackend, projectName string, agents []ProjectAgent) error {
	if config == nil {
		return fmt.Errorf("image ensure config is required")
	}
	for _, agent := range agents {
		driver, err := driverpkg.ResolveSessionRuntimeDriver(agent.Driver, config.RuntimeDriver)
		if err != nil {
			return fmt.Errorf("ensure image for project %s agent %s: %w", projectName, agent.AgentName, err)
		}
		imageRef := driverpkg.ResolveSessionGuestImage(agent.Image, driverpkg.DefaultGuestImageForDriver(config, driver))
		if err := EnsureDriverImage(ctx, backend, DriverEnsureRequest{
			Driver:      driver,
			ImageRef:    imageRef,
			ProjectName: projectName,
			AgentName:   agent.AgentName,
		}); err != nil {
			return err
		}
	}
	return nil
}

func EnsureDriverImage(ctx context.Context, backend ImageBackend, req DriverEnsureRequest) error {
	driver := driverpkg.ResolveRuntimeDriver(req.Driver)
	if driver != driverpkg.RuntimeDriverDocker {
		return nil
	}
	imageRef := strings.TrimSpace(req.ImageRef)
	if imageRef == "" {
		return fmt.Errorf("ensure image for project %s agent %s: driver %s image is required", req.ProjectName, req.AgentName, driver)
	}
	if backend == nil {
		return fmt.Errorf("ensure image for project %s agent %s: driver %s image %s: image backend is required", req.ProjectName, req.AgentName, driver, imageRef)
	}
	if _, err := backend.InspectImage(ctx, ImageInspectRequest{ImageRef: imageRef}); err == nil {
		return nil
	} else if !BackendErrorIsNotFound(err) {
		return fmt.Errorf("ensure image for project %s agent %s: driver %s image %s: %w", req.ProjectName, req.AgentName, driver, imageRef, err)
	}
	if _, err := backend.PullImage(ctx, ImagePullRequest{ImageRef: imageRef}); err != nil {
		return fmt.Errorf("ensure image for project %s agent %s: driver %s image %s: %w", req.ProjectName, req.AgentName, driver, imageRef, err)
	}
	return nil
}

func BackendErrorIsNotFound(err error) bool {
	if err == nil {
		return false
	}
	var backendErr BackendOpError
	if errors.As(err, &backendErr) {
		return cerrdefs.IsNotFound(backendErr.Err)
	}
	return cerrdefs.IsNotFound(err)
}
