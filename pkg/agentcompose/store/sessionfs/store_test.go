package sessionfs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	appconfig "agent-compose/pkg/config"
	driverpkg "agent-compose/pkg/driver"
)

func newTestSessionStore(t *testing.T) *Store {
	t.Helper()
	return NewStoreWithConfig(&appconfig.Config{
		SessionRoot:          filepath.Join(t.TempDir(), "sessions"),
		RuntimeDriver:        driverpkg.RuntimeDriverBoxlite,
		DefaultImage:         "default-box:latest",
		ImageRegistry:        "registry.test",
		GuestHomePath:        "/home/agent-compose",
		MicrosandboxHome:     filepath.Join(t.TempDir(), "microsandbox"),
		JupyterGuestPort:     8888,
		JupyterProxyBasePath: "/agent-compose/session",
	})
}

func TestStoreCreateSessionPersistsMetadataVMAndProxyState(t *testing.T) {
	ctx := context.Background()
	store := newTestSessionStore(t)
	workspace := &SessionWorkspace{ID: "workspace-1", Name: "Workspace", Type: "file", ConfigJSON: "{}"}

	session, err := store.CreateSession(ctx, "", "", driverpkg.RuntimeDriverBoxlite, "", "workspace-1", "script:timer", workspace, []SessionEnvVar{{Name: "PLAIN", Value: "value"}}, []SessionTag{{Name: "kind", Value: "loader"}})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if session.Summary.Title == "" || session.Summary.TriggerSource != "script:timer" || session.Summary.GuestImage != "default-box:latest" {
		t.Fatalf("unexpected session summary: %#v", session.Summary)
	}
	workspace.Name = "mutated"
	loaded, err := store.GetSession(ctx, session.Summary.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if loaded.Workspace == nil || loaded.Workspace.Name != "Workspace" {
		t.Fatalf("workspace was not cloned into session metadata: %#v", loaded.Workspace)
	}

	vmState, err := store.GetVMState(session.Summary.ID)
	if err != nil {
		t.Fatalf("GetVMState: %v", err)
	}
	if vmState.Driver != driverpkg.RuntimeDriverBoxlite || vmState.Registry != "registry.test" || vmState.Image != "default-box:latest" {
		t.Fatalf("vm state = %#v", vmState)
	}
	proxyState, err := store.GetProxyState(session.Summary.ID)
	if err != nil {
		t.Fatalf("GetProxyState: %v", err)
	}
	wantProxyPath := "/agent-compose/session/" + session.Summary.ID + "/lab"
	if proxyState.ProxyPath != wantProxyPath || proxyState.Token == "" || proxyState.HostPort == 0 {
		t.Fatalf("proxy state = %#v, want path %q with token and host port", proxyState, wantProxyPath)
	}
}

func TestStoreCellsEventsAndRunningArtifactsPersist(t *testing.T) {
	ctx := context.Background()
	store := newTestSessionStore(t)
	session, err := store.CreateSession(ctx, "Persistence", "", driverpkg.RuntimeDriverBoxlite, "", "", "", nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	cell := NotebookCell{ID: "cell-1", Type: "shell", Source: "echo first", Stdout: "first\n", Output: "first\n", Success: true, CreatedAt: time.Now().UTC()}
	if err := store.AddCell(ctx, session, cell); err != nil {
		t.Fatalf("AddCell first: %v", err)
	}
	cell.Stdout = "updated\n"
	cell.Output = "updated\n"
	if err := store.AddCell(ctx, session, cell); err != nil {
		t.Fatalf("AddCell update: %v", err)
	}
	runningCell := NotebookCell{ID: "cell-running", Type: "shell", Source: "sleep 1", Running: true, CreatedAt: time.Now().UTC()}
	if err := store.AddCell(ctx, session, runningCell); err != nil {
		t.Fatalf("AddCell running: %v", err)
	}
	cellDir := filepath.Join(store.SessionDir(session.Summary.ID), "state", "cells", runningCell.ID)
	if err := os.MkdirAll(cellDir, 0o755); err != nil {
		t.Fatalf("create running cell dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cellDir, "stdout.txt"), []byte("live stdout\n"), 0o644); err != nil {
		t.Fatalf("write stdout artifact: %v", err)
	}
	if err := os.WriteFile(filepath.Join(cellDir, "stderr.txt"), []byte("live stderr\n"), 0o644); err != nil {
		t.Fatalf("write stderr artifact: %v", err)
	}

	cells, err := store.ListCells(ctx, session.Summary.ID)
	if err != nil {
		t.Fatalf("ListCells: %v", err)
	}
	if len(cells) != 2 || cells[0].Stdout != "updated\n" || cells[1].Stdout != "live stdout\n" || cells[1].Stderr != "live stderr\n" {
		t.Fatalf("cells = %#v", cells)
	}
	if err := store.AddEvent(ctx, session.Summary.ID, SessionEvent{ID: "event-1", Type: "session.tested", Level: "info", Message: "tested", CreatedAt: time.Now().UTC()}); err != nil {
		t.Fatalf("AddEvent: %v", err)
	}
	loaded, err := store.GetSession(ctx, session.Summary.ID)
	if err != nil {
		t.Fatalf("GetSession: %v", err)
	}
	if loaded.Summary.CellCount != 2 || loaded.Summary.EventCount != 1 {
		t.Fatalf("loaded counts = cells %d events %d, want 2/1", loaded.Summary.CellCount, loaded.Summary.EventCount)
	}
}

func TestStoreLegacyAgentRunsMergeIntoCells(t *testing.T) {
	ctx := context.Background()
	store := newTestSessionStore(t)
	session, err := store.CreateSession(ctx, "Legacy", "", driverpkg.RuntimeDriverBoxlite, "", "", "", nil, nil, nil)
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	createdAt := time.Now().UTC().Add(-time.Minute)
	if err := store.AddAgentRun(ctx, session.Summary.ID, AgentRun{
		ID:        "run-1",
		Agent:     "codex",
		Message:   "hello",
		Output:    "world",
		Success:   true,
		CreatedAt: createdAt,
	}); err != nil {
		t.Fatalf("AddAgentRun: %v", err)
	}
	cells, err := store.ListCells(ctx, session.Summary.ID)
	if err != nil {
		t.Fatalf("ListCells: %v", err)
	}
	if len(cells) != 1 || cells[0].ID != "run-1" || cells[0].Type != cellTypeAgent || cells[0].Output != "world" {
		t.Fatalf("merged cells = %#v", cells)
	}
}
