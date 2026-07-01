package agentcompose

import (
	appconfig "agent-compose/pkg/config"
	"context"
	"io"
	"mime/multipart"
	"os"

	workspacehttp "agent-compose/pkg/agentcompose/transport/http/workspace"
	workspacepkg "agent-compose/pkg/agentcompose/workspace"
)

const fileWorkspaceContentDirName = "content"
const gitWorkspaceTempDirName = workspacepkg.GitWorkspaceTempDirName

type fileWorkspaceContent = workspacepkg.FileWorkspaceContent
type gitWorkspaceConfig = workspacepkg.GitConfig
type fileWorkspaceConfig = workspacepkg.FileConfig

func prepareSessionWorkspace(ctx context.Context, config *appconfig.Config, configDB *ConfigStore, session *Session) error {
	wsSession := toWorkspaceSession(session)
	if err := workspacepkg.PrepareSessionWorkspace(ctx, config, configDB, wsSession); err != nil {
		return err
	}
	if session != nil {
		session.WorkspaceID = wsSession.WorkspaceID
	}
	return nil
}

func prepareWorkspaceConfig(ctx context.Context, config *appconfig.Config, session *Session, workspace WorkspaceConfig) error {
	return workspacepkg.PrepareWorkspaceConfig(ctx, config, toWorkspaceSession(session), workspace)
}

func prepareFileWorkspace(config *appconfig.Config, session *Session, workspace WorkspaceConfig) error {
	return workspacepkg.PrepareFileWorkspace(config, toWorkspaceSession(session), workspace)
}

func prepareGitWorkspace(ctx context.Context, session *Session, workspace WorkspaceConfig) error {
	return workspacepkg.PrepareGitWorkspace(ctx, toWorkspaceSession(session), workspace)
}

func fileWorkspaceContentRoot(config *appconfig.Config, workspace WorkspaceConfig) (string, error) {
	return workspacepkg.FileWorkspaceContentRoot(config, workspace)
}

func validateFileWorkspaceConfig(config *appconfig.Config, workspaceID, configJSON string) (string, error) {
	return workspacepkg.ValidateFileWorkspaceConfig(config, workspaceID, configJSON)
}

func openFileWorkspaceContent(config *appconfig.Config, workspace WorkspaceConfig) (fileWorkspaceContent, error) {
	return workspacepkg.OpenFileWorkspaceContent(config, workspace)
}

func fileWorkspaceContentRelRoot(workspaceID string) (string, error) {
	return workspacepkg.FileWorkspaceContentRelRoot(workspaceID)
}

func openFileWorkspaceDataRoot(config *appconfig.Config) (*os.Root, error) {
	return workspacepkg.OpenFileWorkspaceDataRoot(config)
}

func ensureRootParentDir(root *os.Root, relPath string) error {
	return workspacepkg.EnsureRootParentDir(root, relPath)
}

func extractWorkspaceTarArchive(src io.Reader, dstRoot *os.Root) error {
	return workspacepkg.ExtractTarArchive(src, dstRoot)
}

func storeUploadedWorkspaceFile(fileHeader *multipart.FileHeader, contentRoot *os.Root, targetPath string) error {
	return workspacehttp.StoreUploadedFile(fileHeader, contentRoot, targetPath)
}

func copyRootDirectoryContents(srcRoot *os.Root, dstDir string) error {
	return workspacepkg.CopyRootDirectoryContents(srcRoot, dstDir)
}

func normalizeGitCloneTarget(workspaceID, raw string) (string, error) {
	return workspacepkg.NormalizeGitCloneTarget(workspaceID, raw)
}

func moveWorkspaceEntry(src, dst string) error {
	return workspacepkg.MoveWorkspaceEntry(src, dst)
}

func hostWorkspaceInitialized(workspaceRoot string) (bool, error) {
	return workspacepkg.HostWorkspaceInitialized(workspaceRoot)
}

func gitCloneArgs(cloneURL string, cfg gitWorkspaceConfig, clonePath string) []string {
	return workspacepkg.GitCloneArgs(cloneURL, cfg, clonePath)
}

func gitCommitFetchArgs(commit string) []string {
	return workspacepkg.GitCommitFetchArgs(commit)
}

func gitDeepenFetchArgs(unshallow bool) []string {
	return workspacepkg.GitDeepenFetchArgs(unshallow)
}

func applyGitCredentials(cloneURL string, cfg gitWorkspaceConfig) string {
	return workspacepkg.ApplyGitCredentials(cloneURL, cfg)
}

func cleanWorkspaceRelativePath(raw string, allowEmpty bool) (string, error) {
	return workspacepkg.CleanRelativePath(raw, allowEmpty)
}

func defaultFileWorkspaceConfigJSON(config *appconfig.Config, workspaceID string) string {
	return workspacepkg.DefaultFileWorkspaceConfigJSON(config, workspaceID)
}

func defaultFileWorkspaceContentRoot(config *appconfig.Config, workspaceID string) (string, error) {
	return workspacepkg.DefaultFileWorkspaceContentRoot(config, workspaceID)
}

func toWorkspaceSession(session *Session) *workspacepkg.Session {
	if session == nil {
		return nil
	}
	result := &workspacepkg.Session{
		Summary: workspacepkg.SessionSummary{
			ID:            session.Summary.ID,
			WorkspacePath: session.Summary.WorkspacePath,
		},
		WorkspaceID: session.WorkspaceID,
	}
	if session.Workspace != nil {
		result.Workspace = &workspacepkg.SessionWorkspace{
			ID:         session.Workspace.ID,
			Name:       session.Workspace.Name,
			Type:       session.Workspace.Type,
			ConfigJSON: session.Workspace.ConfigJSON,
		}
	}
	return result
}
