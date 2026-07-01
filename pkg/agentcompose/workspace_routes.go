package agentcompose

import (
	"context"
	"fmt"
	"strings"

	workspacehttp "agent-compose/pkg/agentcompose/transport/http/workspace"
	"github.com/labstack/echo/v4"
)

func registerWorkspaceRoutes(app *echo.Echo, service *Service) {
	workspacehttp.RegisterRoutes(app, service)
}

func toWorkspaceUploadHTTPError(err error) error {
	return workspacehttp.ToUploadHTTPError(err)
}

func toWorkspaceHTTPError(err error) error {
	return workspacehttp.ToHTTPError(err)
}

func (s *Service) LoadFileWorkspaceConfig(ctx context.Context, workspaceID string) (WorkspaceConfig, fileWorkspaceContent, error) {
	workspace, err := s.configDB.GetWorkspaceConfig(ctx, workspaceID)
	if err != nil {
		return WorkspaceConfig{}, fileWorkspaceContent{}, err
	}
	if strings.ToLower(strings.TrimSpace(workspace.Type)) != "file" {
		return WorkspaceConfig{}, fileWorkspaceContent{}, fmt.Errorf("workspace config %s is not a file workspace", workspace.ID)
	}
	content, err := openFileWorkspaceContent(s.config, workspace)
	if err != nil {
		return WorkspaceConfig{}, fileWorkspaceContent{}, err
	}
	return workspace, content, nil
}

func (s *Service) loadFileWorkspaceConfig(ctx context.Context, workspaceID string) (WorkspaceConfig, fileWorkspaceContent, error) {
	return s.LoadFileWorkspaceConfig(ctx, workspaceID)
}

func (s *Service) WorkspaceUploadLimitBytes() int64 {
	return s.config.WorkspaceUploadLimitBytes
}
