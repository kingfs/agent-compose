package agentcompose

import (
	"agent-compose/pkg/agentcompose/workspaces"
	appconfig "agent-compose/pkg/config"
	"context"
	"io"
	"mime/multipart"
	"os"
)

const gitWorkspaceTempDirName = workspaces.GitWorkspaceTempDirName

const fileWorkspaceContentDirName = workspaces.FileWorkspaceContentDirName

type gitWorkspaceConfig = workspaces.GitWorkspaceConfig
type fileWorkspaceConfig = workspaces.FileWorkspaceConfig
type fileWorkspaceContent = workspaces.FileWorkspaceContent
type workspaceFileEntry = workspaces.FileEntry

func prepareSessionWorkspace(ctx context.Context, config *appconfig.Config, configDB *ConfigStore, session *Session) error {
	return workspaces.PrepareSessionWorkspace(ctx, config, configDB, session)
}

func prepareFileWorkspace(config *appconfig.Config, session *Session, workspace WorkspaceConfig) error {
	return workspaces.PrepareFileWorkspace(config, session, workspace)
}

func fileWorkspaceContentRoot(config *appconfig.Config, workspace WorkspaceConfig) (string, error) {
	return workspaces.FileWorkspaceContentRoot(config, workspace)
}

func validateFileWorkspaceConfig(config *appconfig.Config, workspaceID, configJSON string) (string, error) {
	return workspaces.ValidateFileWorkspaceConfig(config, workspaceID, configJSON)
}

func openFileWorkspaceContent(config *appconfig.Config, workspace WorkspaceConfig) (fileWorkspaceContent, error) {
	return workspaces.OpenFileWorkspaceContent(config, workspace)
}

func fileWorkspaceContentRelRoot(workspaceID string) (string, error) {
	return workspaces.FileWorkspaceContentRelRoot(workspaceID)
}

func openFileWorkspaceDataRoot(config *appconfig.Config) (*os.Root, error) {
	return workspaces.OpenFileWorkspaceDataRoot(config)
}

func copyRootDirectoryContents(srcRoot *os.Root, dstDir string) error {
	return workspaces.CopyRootDirectoryContents(srcRoot, dstDir)
}

func extractWorkspaceTarArchive(src io.Reader, dstRoot *os.Root) error {
	return workspaces.ExtractWorkspaceTarArchive(src, dstRoot)
}

func hostWorkspaceInitialized(workspaceRoot string) (bool, error) {
	return workspaces.HostWorkspaceInitialized(workspaceRoot)
}

func moveWorkspaceEntry(src, dst string) error {
	return workspaces.MoveWorkspaceEntry(src, dst)
}

func prepareGitWorkspace(ctx context.Context, session *Session, workspace WorkspaceConfig) error {
	return workspaces.PrepareGitWorkspace(ctx, session, workspace)
}

func normalizeGitCloneTarget(workspaceID, raw string) (string, error) {
	return workspaces.NormalizeGitCloneTarget(workspaceID, raw)
}

func gitCloneArgs(cloneURL string, cfg gitWorkspaceConfig, clonePath string) []string {
	return workspaces.GitCloneArgs(cloneURL, cfg, clonePath)
}

func gitCommitFetchArgs(commit string) []string {
	return workspaces.GitCommitFetchArgs(commit)
}

func gitDeepenFetchArgs(unshallow bool) []string {
	return workspaces.GitDeepenFetchArgs(unshallow)
}

func applyGitCredentials(cloneURL string, cfg gitWorkspaceConfig) string {
	return workspaces.ApplyGitCredentials(cloneURL, cfg)
}

func listWorkspaceFiles(contentRoot *os.Root) ([]workspaceFileEntry, error) {
	return workspaces.ListFiles(contentRoot)
}

func cleanWorkspaceRelativePath(raw string, allowEmpty bool) (string, error) {
	return workspaces.CleanRelativePath(raw, allowEmpty)
}

func storeUploadedWorkspaceFile(fileHeader *multipart.FileHeader, contentRoot *os.Root, targetPath string) error {
	return workspaces.StoreUploadedFile(fileHeader, contentRoot, targetPath)
}

func extractUploadedWorkspaceArchive(fileHeader *multipart.FileHeader, contentRoot *os.Root) error {
	return workspaces.ExtractUploadedArchive(fileHeader, contentRoot)
}

func defaultFileWorkspaceConfigJSON(config *appconfig.Config, workspaceID string) string {
	return workspaces.DefaultFileConfigJSON(config, workspaceID)
}

func defaultFileWorkspaceContentRoot(config *appconfig.Config, workspaceID string) (string, error) {
	return workspaces.DefaultFileWorkspaceContentRoot(config, workspaceID)
}
