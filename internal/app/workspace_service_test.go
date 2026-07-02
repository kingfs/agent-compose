package app

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"connectrpc.com/connect"
	"github.com/labstack/echo/v4"

	appconfig "agent-compose/pkg/config"
	agentcomposev1 "agent-compose/proto/agentcompose/v1"
)

func TestRegisterWorkspaceRoutesUploadAndList(t *testing.T) {
	testRegisterWorkspaceRoutesUploadAndList(t)
}

func testRegisterWorkspaceRoutesUploadAndList(t *testing.T) {
	ctx := context.Background()
	config := &appconfig.Config{DataRoot: t.TempDir()}
	configDB := newWorkspaceRouteTestConfigStore(t)
	workspaceID := "ws-file"
	workspace, err := configDB.CreateWorkspaceConfig(ctx, WorkspaceConfig{
		ID:         workspaceID,
		Name:       "File Workspace",
		Type:       "file",
		ConfigJSON: defaultFileWorkspaceConfigJSON(config, workspaceID),
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceConfig returned error: %v", err)
	}
	service := &Service{config: config, configDB: configDB}
	e := echo.New()
	registerWorkspaceRoutes(e, service)

	archiveBody := &bytes.Buffer{}
	archiveWriter := multipart.NewWriter(archiveBody)
	archivePart, err := archiveWriter.CreateFormFile("file", "workspace.tar")
	if err != nil {
		t.Fatalf("CreateFormFile archive: %v", err)
	}
	writeTestTar(t, archivePart, map[string]string{"nested/file.txt": "archive\n"})
	if err := archiveWriter.WriteField("upload_type", "archive"); err != nil {
		t.Fatalf("WriteField upload_type archive: %v", err)
	}
	if err := archiveWriter.Close(); err != nil {
		t.Fatalf("close archive writer: %v", err)
	}
	archiveReq := httptest.NewRequest(http.MethodPost, "/api/agent-compose/workspaces/"+workspace.ID+"/upload", archiveBody)
	archiveReq.Header.Set(echo.HeaderContentType, archiveWriter.FormDataContentType())
	archiveRec := httptest.NewRecorder()
	e.ServeHTTP(archiveRec, archiveReq)
	if archiveRec.Code != http.StatusOK {
		t.Fatalf("archive upload status = %d, body=%s", archiveRec.Code, archiveRec.Body.String())
	}

	fileBody := &bytes.Buffer{}
	fileWriter := multipart.NewWriter(fileBody)
	filePart, err := fileWriter.CreateFormFile("file", "notes.txt")
	if err != nil {
		t.Fatalf("CreateFormFile single file: %v", err)
	}
	if _, err := filePart.Write([]byte("notes\n")); err != nil {
		t.Fatalf("write single file content: %v", err)
	}
	if err := fileWriter.WriteField("upload_type", "file"); err != nil {
		t.Fatalf("WriteField upload_type file: %v", err)
	}
	if err := fileWriter.WriteField("path", "docs/notes.txt"); err != nil {
		t.Fatalf("WriteField path: %v", err)
	}
	if err := fileWriter.Close(); err != nil {
		t.Fatalf("close file writer: %v", err)
	}
	fileReq := httptest.NewRequest(http.MethodPost, "/api/agent-compose/workspaces/"+workspace.ID+"/upload", fileBody)
	fileReq.Header.Set(echo.HeaderContentType, fileWriter.FormDataContentType())
	fileRec := httptest.NewRecorder()
	e.ServeHTTP(fileRec, fileReq)
	if fileRec.Code != http.StatusOK {
		t.Fatalf("single file upload status = %d, body=%s", fileRec.Code, fileRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/agent-compose/workspaces/"+workspace.ID+"/files", nil)
	listRec := httptest.NewRecorder()
	e.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusOK {
		t.Fatalf("list files status = %d, body=%s", listRec.Code, listRec.Body.String())
	}
	assertFileContent(t, filepath.Join(mustFileWorkspaceContentRoot(t, config, workspace), "nested", "file.txt"), "archive\n")
	assertFileContent(t, filepath.Join(mustFileWorkspaceContentRoot(t, config, workspace), "docs", "notes.txt"), "notes\n")
	if !bytes.Contains(listRec.Body.Bytes(), []byte(`"path":"docs/notes.txt"`)) {
		t.Fatalf("expected listed file docs/notes.txt, body=%s", listRec.Body.String())
	}
	if !bytes.Contains(listRec.Body.Bytes(), []byte(`"path":"nested/file.txt"`)) {
		t.Fatalf("expected listed file nested/file.txt, body=%s", listRec.Body.String())
	}
}

func TestCreateFileWorkspaceConfigDefaultRootFromEmptyObject(t *testing.T) {
	ctx := context.Background()
	config := &appconfig.Config{DataRoot: t.TempDir()}
	configDB := newWorkspaceRouteTestConfigStore(t)
	service := &Service{config: config, configDB: configDB}
	resp, err := service.CreateWorkspaceConfig(ctx, connect.NewRequest(&agentcomposev1.CreateWorkspaceConfigRequest{
		Name:       "File Workspace",
		Type:       "file",
		ConfigJson: "{}",
	}))
	if err != nil {
		t.Fatalf("CreateWorkspaceConfig returned error: %v", err)
	}
	workspace := resp.Msg.GetWorkspace()
	if workspace.GetId() == "" {
		t.Fatalf("expected generated workspace id")
	}
	var cfg fileWorkspaceConfig
	if err := json.Unmarshal([]byte(workspace.GetConfigJson()), &cfg); err != nil {
		t.Fatalf("decode config json: %v", err)
	}
	wantRoot := mustDefaultFileWorkspaceContentRoot(t, config, workspace.GetId())
	if cfg.Root != wantRoot {
		t.Fatalf("root = %q, want %q", cfg.Root, wantRoot)
	}
	if _, err := os.Stat(wantRoot); err != nil {
		t.Fatalf("expected default root to be created: %v", err)
	}
}

func TestLoadLegacyFileWorkspaceConfigDefaultRootFromEmptyObject(t *testing.T) {
	ctx := context.Background()
	config := &appconfig.Config{DataRoot: t.TempDir()}
	configDB := newWorkspaceRouteTestConfigStore(t)
	service := &Service{config: config, configDB: configDB}
	workspaceID := "ws-file"
	_, err := configDB.CreateWorkspaceConfig(ctx, WorkspaceConfig{
		ID:         workspaceID,
		Name:       "File Workspace",
		Type:       "file",
		ConfigJSON: "{}",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceConfig returned error: %v", err)
	}
	_, content, err := service.loadFileWorkspaceConfig(ctx, workspaceID)
	if err != nil {
		t.Fatalf("loadFileWorkspaceConfig returned error: %v", err)
	}
	defer func() { _ = content.Root.Close() }()
	wantRoot := mustDefaultFileWorkspaceContentRoot(t, config, workspaceID)
	if content.AbsRoot != wantRoot {
		t.Fatalf("content root = %q, want %q", content.AbsRoot, wantRoot)
	}
	if _, err := os.Stat(wantRoot); err != nil {
		t.Fatalf("expected default root to be created: %v", err)
	}
}

func TestCreateFileWorkspaceConfigDefaultRootFromRelativeDataRoot(t *testing.T) {
	ctx := context.Background()
	cwd := t.TempDir()
	oldwd, err := os.Getwd()
	if err != nil {
		t.Fatalf("Getwd: %v", err)
	}
	if err := os.Chdir(cwd); err != nil {
		t.Fatalf("Chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(oldwd) })
	config := &appconfig.Config{DataRoot: filepath.Join(".", "data", "agent-compose")}
	configDB := newWorkspaceRouteTestConfigStore(t)
	service := &Service{config: config, configDB: configDB}
	resp, err := service.CreateWorkspaceConfig(ctx, connect.NewRequest(&agentcomposev1.CreateWorkspaceConfigRequest{
		Name: "File Workspace",
		Type: "file",
	}))
	if err != nil {
		t.Fatalf("CreateWorkspaceConfig returned error: %v", err)
	}
	var cfg fileWorkspaceConfig
	if err := json.Unmarshal([]byte(resp.Msg.GetWorkspace().GetConfigJson()), &cfg); err != nil {
		t.Fatalf("decode config json: %v", err)
	}
	if !filepath.IsAbs(cfg.Root) {
		t.Fatalf("expected absolute root, got %q", cfg.Root)
	}
}

func TestCreateFileWorkspaceConfigOverridesClientRoot(t *testing.T) {
	ctx := context.Background()
	config := &appconfig.Config{DataRoot: t.TempDir()}
	configDB := newWorkspaceRouteTestConfigStore(t)
	service := &Service{config: config, configDB: configDB}
	resp, err := service.CreateWorkspaceConfig(ctx, connect.NewRequest(&agentcomposev1.CreateWorkspaceConfigRequest{
		Name:       "File Workspace",
		Type:       "file",
		ConfigJson: encodeFileWorkspaceConfigForTest(t, t.TempDir()),
	}))
	if err != nil {
		t.Fatalf("CreateWorkspaceConfig returned error: %v", err)
	}
	workspace := resp.Msg.GetWorkspace()
	var cfg fileWorkspaceConfig
	if err := json.Unmarshal([]byte(workspace.GetConfigJson()), &cfg); err != nil {
		t.Fatalf("decode config json: %v", err)
	}
	wantRoot := mustDefaultFileWorkspaceContentRoot(t, config, workspace.GetId())
	if cfg.Root != wantRoot {
		t.Fatalf("root = %q, want service root %q", cfg.Root, wantRoot)
	}
}

func TestCreateFileWorkspaceConfigOverridesOtherWorkspaceRoot(t *testing.T) {
	ctx := context.Background()
	config := &appconfig.Config{DataRoot: t.TempDir()}
	configDB := newWorkspaceRouteTestConfigStore(t)
	service := &Service{config: config, configDB: configDB}
	otherRoot := mustDefaultFileWorkspaceContentRoot(t, config, "other-workspace")
	resp, err := service.CreateWorkspaceConfig(ctx, connect.NewRequest(&agentcomposev1.CreateWorkspaceConfigRequest{
		Name:       "File Workspace",
		Type:       "file",
		ConfigJson: encodeFileWorkspaceConfigForTest(t, otherRoot),
	}))
	if err != nil {
		t.Fatalf("CreateWorkspaceConfig returned error: %v", err)
	}
	workspace := resp.Msg.GetWorkspace()
	var cfg fileWorkspaceConfig
	if err := json.Unmarshal([]byte(workspace.GetConfigJson()), &cfg); err != nil {
		t.Fatalf("decode config json: %v", err)
	}
	wantRoot := mustDefaultFileWorkspaceContentRoot(t, config, workspace.GetId())
	if cfg.Root != wantRoot {
		t.Fatalf("root = %q, want service root %q", cfg.Root, wantRoot)
	}
}

func TestUpdateFileWorkspaceConfigOverridesClientRoot(t *testing.T) {
	ctx := context.Background()
	config := &appconfig.Config{DataRoot: t.TempDir()}
	configDB := newWorkspaceRouteTestConfigStore(t)
	service := &Service{config: config, configDB: configDB}
	created, err := service.CreateWorkspaceConfig(ctx, connect.NewRequest(&agentcomposev1.CreateWorkspaceConfigRequest{
		Name: "File Workspace",
		Type: "file",
	}))
	if err != nil {
		t.Fatalf("CreateWorkspaceConfig returned error: %v", err)
	}
	workspaceID := created.Msg.GetWorkspace().GetId()
	updated, err := service.UpdateWorkspaceConfig(ctx, connect.NewRequest(&agentcomposev1.UpdateWorkspaceConfigRequest{
		WorkspaceId: workspaceID,
		Name:        "Updated File Workspace",
		Type:        "file",
		ConfigJson:  encodeFileWorkspaceConfigForTest(t, t.TempDir()),
	}))
	if err != nil {
		t.Fatalf("UpdateWorkspaceConfig returned error: %v", err)
	}
	loaded, err := configDB.GetWorkspaceConfig(ctx, workspaceID)
	if err != nil {
		t.Fatalf("GetWorkspaceConfig returned error: %v", err)
	}
	if loaded.Name != "Updated File Workspace" {
		t.Fatalf("workspace name = %q, want updated name", loaded.Name)
	}
	if loaded.ConfigJSON != updated.Msg.GetWorkspace().GetConfigJson() {
		t.Fatalf("stored config %q differs from response config %q", loaded.ConfigJSON, updated.Msg.GetWorkspace().GetConfigJson())
	}
	var cfg fileWorkspaceConfig
	if err := json.Unmarshal([]byte(loaded.ConfigJSON), &cfg); err != nil {
		t.Fatalf("decode config json: %v", err)
	}
	wantRoot := mustDefaultFileWorkspaceContentRoot(t, config, workspaceID)
	if cfg.Root != wantRoot {
		t.Fatalf("root = %q, want service root %q", cfg.Root, wantRoot)
	}
}

func TestUpdateFileWorkspaceConfigFileToGitRemovesContent(t *testing.T) {
	ctx := context.Background()
	config := &appconfig.Config{DataRoot: t.TempDir()}
	configDB := newWorkspaceRouteTestConfigStore(t)
	service := &Service{config: config, configDB: configDB}
	created, err := service.CreateWorkspaceConfig(ctx, connect.NewRequest(&agentcomposev1.CreateWorkspaceConfigRequest{
		Name: "File Workspace",
		Type: "file",
	}))
	if err != nil {
		t.Fatalf("CreateWorkspaceConfig returned error: %v", err)
	}
	workspaceID := created.Msg.GetWorkspace().GetId()
	contentRoot := mustDefaultFileWorkspaceContentRoot(t, config, workspaceID)
	if err := os.WriteFile(filepath.Join(contentRoot, "data.txt"), []byte("data\n"), 0o644); err != nil {
		t.Fatalf("write content file: %v", err)
	}
	_, err = service.UpdateWorkspaceConfig(ctx, connect.NewRequest(&agentcomposev1.UpdateWorkspaceConfigRequest{
		WorkspaceId: workspaceID,
		Name:        "Git Workspace",
		Type:        "git",
		ConfigJson:  `{"url":"https://example.test/repo.git"}`,
	}))
	if err != nil {
		t.Fatalf("UpdateWorkspaceConfig returned error: %v", err)
	}
	if _, err := os.Stat(contentRoot); !os.IsNotExist(err) {
		t.Fatalf("expected content root to be removed, stat err=%v", err)
	}
}

func TestUpdateFileWorkspaceConfigFileToFileKeepsContent(t *testing.T) {
	ctx := context.Background()
	config := &appconfig.Config{DataRoot: t.TempDir()}
	configDB := newWorkspaceRouteTestConfigStore(t)
	service := &Service{config: config, configDB: configDB}
	created, err := service.CreateWorkspaceConfig(ctx, connect.NewRequest(&agentcomposev1.CreateWorkspaceConfigRequest{
		Name: "File Workspace",
		Type: "file",
	}))
	if err != nil {
		t.Fatalf("CreateWorkspaceConfig returned error: %v", err)
	}
	workspaceID := created.Msg.GetWorkspace().GetId()
	contentRoot := mustDefaultFileWorkspaceContentRoot(t, config, workspaceID)
	contentPath := filepath.Join(contentRoot, "data.txt")
	if err := os.WriteFile(contentPath, []byte("data\n"), 0o644); err != nil {
		t.Fatalf("write content file: %v", err)
	}
	_, err = service.UpdateWorkspaceConfig(ctx, connect.NewRequest(&agentcomposev1.UpdateWorkspaceConfigRequest{
		WorkspaceId: workspaceID,
		Name:        "Updated File Workspace",
		Type:        "file",
	}))
	if err != nil {
		t.Fatalf("UpdateWorkspaceConfig returned error: %v", err)
	}
	assertFileContent(t, contentPath, "data\n")
}

func TestUpdateFileWorkspaceConfigFileToGitKeepsConfigWhenContentRemovalFails(t *testing.T) {
	ctx := context.Background()
	config := &appconfig.Config{DataRoot: t.TempDir()}
	configDB := newWorkspaceRouteTestConfigStore(t)
	service := &Service{config: config, configDB: configDB}
	created, err := service.CreateWorkspaceConfig(ctx, connect.NewRequest(&agentcomposev1.CreateWorkspaceConfigRequest{
		Name: "File Workspace",
		Type: "file",
	}))
	if err != nil {
		t.Fatalf("CreateWorkspaceConfig returned error: %v", err)
	}
	workspaceID := created.Msg.GetWorkspace().GetId()
	contentRoot := mustDefaultFileWorkspaceContentRoot(t, config, workspaceID)
	if err := os.RemoveAll(contentRoot); err != nil {
		t.Fatalf("RemoveAll content root: %v", err)
	}
	if err := os.Symlink(t.TempDir(), contentRoot); err != nil {
		t.Fatalf("create content symlink: %v", err)
	}
	_, err = service.UpdateWorkspaceConfig(ctx, connect.NewRequest(&agentcomposev1.UpdateWorkspaceConfigRequest{
		WorkspaceId: workspaceID,
		Name:        "Git Workspace",
		Type:        "git",
		ConfigJson:  `{"url":"https://example.test/repo.git"}`,
	}))
	if err == nil {
		t.Fatalf("expected UpdateWorkspaceConfig to fail when file content removal fails")
	}
	loaded, err := configDB.GetWorkspaceConfig(ctx, workspaceID)
	if err != nil {
		t.Fatalf("GetWorkspaceConfig returned error: %v", err)
	}
	if loaded.Type != "file" {
		t.Fatalf("workspace type = %q, want file after failed update", loaded.Type)
	}
}

func TestDeleteFileWorkspaceConfigKeepsConfigWhenContentRemovalFails(t *testing.T) {
	ctx := context.Background()
	config := &appconfig.Config{DataRoot: t.TempDir()}
	configDB := newWorkspaceRouteTestConfigStore(t)
	service := &Service{config: config, configDB: configDB}
	created, err := service.CreateWorkspaceConfig(ctx, connect.NewRequest(&agentcomposev1.CreateWorkspaceConfigRequest{
		Name: "File Workspace",
		Type: "file",
	}))
	if err != nil {
		t.Fatalf("CreateWorkspaceConfig returned error: %v", err)
	}
	workspaceID := created.Msg.GetWorkspace().GetId()
	contentRoot := mustDefaultFileWorkspaceContentRoot(t, config, workspaceID)
	if err := os.RemoveAll(contentRoot); err != nil {
		t.Fatalf("RemoveAll content root: %v", err)
	}
	if err := os.Symlink(t.TempDir(), contentRoot); err != nil {
		t.Fatalf("create content symlink: %v", err)
	}
	_, err = service.DeleteWorkspaceConfig(ctx, connect.NewRequest(&agentcomposev1.WorkspaceConfigIDRequest{
		WorkspaceId: workspaceID,
	}))
	if err == nil {
		t.Fatalf("expected DeleteWorkspaceConfig to fail when file content removal fails")
	}
	if _, err := configDB.GetWorkspaceConfig(ctx, workspaceID); err != nil {
		t.Fatalf("expected workspace config to remain after failed delete, got error: %v", err)
	}
}

func TestDeleteFileWorkspaceConfigRemovesContentWithInvalidStoredConfig(t *testing.T) {
	ctx := context.Background()
	config := &appconfig.Config{DataRoot: t.TempDir()}
	configDB := newWorkspaceRouteTestConfigStore(t)
	service := &Service{config: config, configDB: configDB}
	workspaceID := "ws-file"
	contentRoot := mustDefaultFileWorkspaceContentRoot(t, config, workspaceID)
	if err := os.MkdirAll(contentRoot, 0o755); err != nil {
		t.Fatalf("mkdir content root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contentRoot, "data.txt"), []byte("data\n"), 0o644); err != nil {
		t.Fatalf("write content file: %v", err)
	}
	_, err := configDB.CreateWorkspaceConfig(ctx, WorkspaceConfig{
		ID:         workspaceID,
		Name:       "File Workspace",
		Type:       "file",
		ConfigJSON: "{",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceConfig returned error: %v", err)
	}
	_, err = service.DeleteWorkspaceConfig(ctx, connect.NewRequest(&agentcomposev1.WorkspaceConfigIDRequest{
		WorkspaceId: workspaceID,
	}))
	if err != nil {
		t.Fatalf("DeleteWorkspaceConfig returned error: %v", err)
	}
	if _, err := os.Stat(contentRoot); !os.IsNotExist(err) {
		t.Fatalf("expected content root to be removed despite invalid stored config, stat err=%v", err)
	}
}

func TestUpdateFileWorkspaceConfigFileToGitRemovesContentWithInvalidStoredConfig(t *testing.T) {
	ctx := context.Background()
	config := &appconfig.Config{DataRoot: t.TempDir()}
	configDB := newWorkspaceRouteTestConfigStore(t)
	service := &Service{config: config, configDB: configDB}
	workspaceID := "ws-file"
	contentRoot := mustDefaultFileWorkspaceContentRoot(t, config, workspaceID)
	if err := os.MkdirAll(contentRoot, 0o755); err != nil {
		t.Fatalf("mkdir content root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contentRoot, "data.txt"), []byte("data\n"), 0o644); err != nil {
		t.Fatalf("write content file: %v", err)
	}
	_, err := configDB.CreateWorkspaceConfig(ctx, WorkspaceConfig{
		ID:         workspaceID,
		Name:       "File Workspace",
		Type:       "file",
		ConfigJSON: "{",
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceConfig returned error: %v", err)
	}
	_, err = service.UpdateWorkspaceConfig(ctx, connect.NewRequest(&agentcomposev1.UpdateWorkspaceConfigRequest{
		WorkspaceId: workspaceID,
		Name:        "Git Workspace",
		Type:        "git",
		ConfigJson:  `{"url":"https://example.test/repo.git"}`,
	}))
	if err != nil {
		t.Fatalf("UpdateWorkspaceConfig returned error: %v", err)
	}
	if _, err := os.Stat(contentRoot); !os.IsNotExist(err) {
		t.Fatalf("expected content root to be removed despite invalid stored config, stat err=%v", err)
	}
}

func TestWorkspaceRoutesUploadRejectsBodyOverLimit(t *testing.T) {
	ctx := context.Background()
	config := &appconfig.Config{DataRoot: t.TempDir(), WorkspaceUploadLimitBytes: 128}
	configDB := newWorkspaceRouteTestConfigStore(t)
	service := &Service{config: config, configDB: configDB}
	created, err := service.CreateWorkspaceConfig(ctx, connect.NewRequest(&agentcomposev1.CreateWorkspaceConfigRequest{
		Name: "File Workspace",
		Type: "file",
	}))
	if err != nil {
		t.Fatalf("CreateWorkspaceConfig returned error: %v", err)
	}
	workspaceID := created.Msg.GetWorkspace().GetId()
	e := echo.New()
	registerWorkspaceRoutes(e, service)
	body := &bytes.Buffer{}
	writer := multipart.NewWriter(body)
	part, err := writer.CreateFormFile("file", "big.txt")
	if err != nil {
		t.Fatalf("CreateFormFile: %v", err)
	}
	if _, err := part.Write(bytes.Repeat([]byte("x"), 1024)); err != nil {
		t.Fatalf("write body: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("close writer: %v", err)
	}
	req := httptest.NewRequest(http.MethodPost, "/api/agent-compose/workspaces/"+workspaceID+"/upload", body)
	req.Header.Set(echo.HeaderContentType, writer.FormDataContentType())
	rec := httptest.NewRecorder()
	e.ServeHTTP(rec, req)
	if rec.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("upload status = %d, want %d, body=%s", rec.Code, http.StatusRequestEntityTooLarge, rec.Body.String())
	}
	workspace, err := configDB.GetWorkspaceConfig(ctx, workspaceID)
	if err != nil {
		t.Fatalf("GetWorkspaceConfig returned error: %v", err)
	}
	contentRoot := mustFileWorkspaceContentRoot(t, config, workspace)
	if _, err := os.Stat(filepath.Join(contentRoot, "big.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected oversized upload target to be absent, stat err=%v", err)
	}
}

func TestWorkspaceRoutesRejectSymlinkListAndDownload(t *testing.T) {
	ctx := context.Background()
	config := &appconfig.Config{DataRoot: t.TempDir()}
	configDB := newWorkspaceRouteTestConfigStore(t)
	service := &Service{config: config, configDB: configDB}
	created, err := service.CreateWorkspaceConfig(ctx, connect.NewRequest(&agentcomposev1.CreateWorkspaceConfigRequest{
		Name: "File Workspace",
		Type: "file",
	}))
	if err != nil {
		t.Fatalf("CreateWorkspaceConfig returned error: %v", err)
	}
	workspaceID := created.Msg.GetWorkspace().GetId()
	workspace, err := configDB.GetWorkspaceConfig(ctx, workspaceID)
	if err != nil {
		t.Fatalf("GetWorkspaceConfig returned error: %v", err)
	}
	contentRoot := mustFileWorkspaceContentRoot(t, config, workspace)
	outsideFile := filepath.Join(t.TempDir(), "outside.txt")
	if err := os.WriteFile(outsideFile, []byte("outside\n"), 0o644); err != nil {
		t.Fatalf("write outside file: %v", err)
	}
	if err := os.Symlink(outsideFile, filepath.Join(contentRoot, "link.txt")); err != nil {
		t.Fatalf("create symlink: %v", err)
	}
	e := echo.New()
	registerWorkspaceRoutes(e, service)
	listReq := httptest.NewRequest(http.MethodGet, "/api/agent-compose/workspaces/"+workspaceID+"/files", nil)
	listRec := httptest.NewRecorder()
	e.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusInternalServerError {
		t.Fatalf("list files status = %d, body=%s", listRec.Code, listRec.Body.String())
	}
	downloadReq := httptest.NewRequest(http.MethodGet, "/api/agent-compose/workspaces/"+workspaceID+"/download?path=link.txt", nil)
	downloadRec := httptest.NewRecorder()
	e.ServeHTTP(downloadRec, downloadReq)
	if downloadRec.Code != http.StatusBadRequest {
		t.Fatalf("download symlink status = %d, body=%s", downloadRec.Code, downloadRec.Body.String())
	}
}

func TestWorkspaceRoutesRejectSymlinkContentRoot(t *testing.T) {
	ctx := context.Background()
	config := &appconfig.Config{DataRoot: t.TempDir()}
	configDB := newWorkspaceRouteTestConfigStore(t)
	service := &Service{config: config, configDB: configDB}
	workspaceID := "ws-file"
	workspaceRoot := filepath.Join(config.DataRoot, "workspaces", workspaceID)
	if err := os.MkdirAll(workspaceRoot, 0o755); err != nil {
		t.Fatalf("mkdir workspace root: %v", err)
	}
	outsideRoot := t.TempDir()
	if err := os.Symlink(outsideRoot, filepath.Join(workspaceRoot, fileWorkspaceContentDirName)); err != nil {
		t.Fatalf("create content symlink: %v", err)
	}
	_, err := configDB.CreateWorkspaceConfig(ctx, WorkspaceConfig{
		ID:         workspaceID,
		Name:       "File Workspace",
		Type:       "file",
		ConfigJSON: defaultFileWorkspaceConfigJSON(config, workspaceID),
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceConfig returned error: %v", err)
	}
	e := echo.New()
	registerWorkspaceRoutes(e, service)
	listReq := httptest.NewRequest(http.MethodGet, "/api/agent-compose/workspaces/"+workspaceID+"/files", nil)
	listRec := httptest.NewRecorder()
	e.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusInternalServerError {
		t.Fatalf("list files status = %d, body=%s", listRec.Code, listRec.Body.String())
	}
}

func TestWorkspaceRoutesRejectSymlinkDataRoot(t *testing.T) {
	ctx := context.Background()
	realDataRoot := t.TempDir()
	linkRoot := filepath.Join(t.TempDir(), "data-link")
	if err := os.Symlink(realDataRoot, linkRoot); err != nil {
		t.Fatalf("create data root symlink: %v", err)
	}
	config := &appconfig.Config{DataRoot: linkRoot}
	configDB := newWorkspaceRouteTestConfigStore(t)
	service := &Service{config: config, configDB: configDB}
	workspaceID := "ws-file"
	_, err := configDB.CreateWorkspaceConfig(ctx, WorkspaceConfig{
		ID:         workspaceID,
		Name:       "File Workspace",
		Type:       "file",
		ConfigJSON: defaultFileWorkspaceConfigJSON(config, workspaceID),
	})
	if err != nil {
		t.Fatalf("CreateWorkspaceConfig returned error: %v", err)
	}
	e := echo.New()
	registerWorkspaceRoutes(e, service)
	listReq := httptest.NewRequest(http.MethodGet, "/api/agent-compose/workspaces/"+workspaceID+"/files", nil)
	listRec := httptest.NewRecorder()
	e.ServeHTTP(listRec, listReq)
	if listRec.Code != http.StatusInternalServerError {
		t.Fatalf("list files status = %d, body=%s", listRec.Code, listRec.Body.String())
	}
}

func newWorkspaceRouteTestConfigStore(t *testing.T) *ConfigStore {
	t.Helper()
	return newTestConfigStore(t)
}

func encodeFileWorkspaceConfigForTest(t *testing.T, root string) string {
	t.Helper()
	payload, err := json.Marshal(fileWorkspaceConfig{Root: root})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(payload)
}

func mustFileWorkspaceContentRoot(t *testing.T, config *appconfig.Config, workspace WorkspaceConfig) string {
	t.Helper()
	root, err := fileWorkspaceContentRoot(config, workspace)
	if err != nil {
		t.Fatalf("fileWorkspaceContentRoot: %v", err)
	}
	return root
}

func mustDefaultFileWorkspaceContentRoot(t *testing.T, config *appconfig.Config, workspaceID string) string {
	t.Helper()
	root, err := defaultFileWorkspaceContentRoot(config, workspaceID)
	if err != nil {
		t.Fatalf("defaultFileWorkspaceContentRoot: %v", err)
	}
	return root
}

func writeTestTar(t *testing.T, dst io.Writer, files map[string]string) {
	t.Helper()
	tw := tar.NewWriter(dst)
	for name, body := range files {
		if err := tw.WriteHeader(&tar.Header{Name: name, Mode: 0o644, Size: int64(len(body))}); err != nil {
			t.Fatalf("WriteHeader %s: %v", name, err)
		}
		if _, err := tw.Write([]byte(body)); err != nil {
			t.Fatalf("Write body %s: %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
}
