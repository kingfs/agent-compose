package app

import (
	"bytes"
	"mime/multipart"
	"os"
	"path/filepath"
	"strings"
	"testing"

	workspacedomain "agent-compose/internal/workspace"
	appconfig "agent-compose/pkg/config"
)

func writeUploadedWorkspaceFileForTest(contentRoot, targetPath, body string) error {
	root, err := os.OpenRoot(contentRoot)
	if err != nil {
		return err
	}
	defer func() { _ = root.Close() }()
	var formBody bytes.Buffer
	writer := multipart.NewWriter(&formBody)
	part, err := writer.CreateFormFile("file", "upload.txt")
	if err != nil {
		return err
	}
	if _, err := part.Write([]byte(body)); err != nil {
		return err
	}
	if err := writer.Close(); err != nil {
		return err
	}
	reader := multipart.NewReader(&formBody, strings.TrimPrefix(writer.FormDataContentType(), "multipart/form-data; boundary="))
	form, err := reader.ReadForm(1024 * 1024)
	if err != nil {
		return err
	}
	defer func() { _ = form.RemoveAll() }()
	return storeUploadedWorkspaceFile(form.File["file"][0], root, targetPath)
}

func TestStoreUploadedWorkspaceFileRejectsSymlinkParent(t *testing.T) {
	contentRoot := t.TempDir()
	outsideRoot := t.TempDir()
	if err := os.Symlink(outsideRoot, filepath.Join(contentRoot, "link")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	if err := writeUploadedWorkspaceFileForTest(contentRoot, "link/owned.txt", "escape\n"); err == nil {
		t.Fatalf("expected symlink parent upload target to be rejected")
	}
	if _, err := os.Stat(filepath.Join(outsideRoot, "owned.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected outside file to be absent, stat err=%v", err)
	}
}

func TestPrepareFileWorkspaceRejectsSymlinkContent(t *testing.T) {
	config := &appconfig.Config{DataRoot: t.TempDir()}
	contentRoot := filepath.Join(config.DataRoot, "workspaces", "ws-file", fileWorkspaceContentDirName)
	if err := os.MkdirAll(contentRoot, 0o755); err != nil {
		t.Fatalf("mkdir content root: %v", err)
	}
	if err := os.Symlink("/tmp", filepath.Join(contentRoot, "link")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	session := &Session{Summary: SessionSummary{ID: "session-1", WorkspacePath: t.TempDir()}}
	workspace := WorkspaceConfig{
		ID:         "ws-file",
		Name:       "File Workspace",
		Type:       "file",
		ConfigJSON: encodeFileWorkspaceConfigForTest(t, contentRoot),
	}
	if err := workspacedomain.PrepareFileWorkspace(config, session, workspace); err == nil {
		t.Fatalf("expected file workspace symlink content to be rejected")
	}
}
