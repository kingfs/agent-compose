package workspace

import (
	appconfig "agent-compose/pkg/config"
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func TestNormalizeGitCloneTarget(t *testing.T) {
	tests := []struct {
		name    string
		raw     string
		want    string
		wantErr bool
	}{
		{name: "default root", raw: "", want: "."},
		{name: "relative path", raw: "repo/subdir", want: filepath.Clean("repo/subdir")},
		{name: "collapse clean path", raw: "repo/../src", want: "src"},
		{name: "reject absolute", raw: "/tmp/repo", wantErr: true},
		{name: "reject parent", raw: "../repo", wantErr: true},
		{name: "reject cleaned parent", raw: "repo/../../escape", wantErr: true},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := normalizeGitCloneTarget("ws-1", tc.raw)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error, got target %q", got)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeGitCloneTarget returned error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("normalizeGitCloneTarget = %q, want %q", got, tc.want)
			}
		})
	}
}

func TestHostWorkspaceInitializedIgnoresInternalEntries(t *testing.T) {
	workspaceRoot := t.TempDir()
	if err := os.Mkdir(filepath.Join(workspaceRoot, ".agent-compose"), 0o755); err != nil {
		t.Fatalf("mkdir .agent-compose: %v", err)
	}
	if err := os.Mkdir(filepath.Join(workspaceRoot, gitWorkspaceTempDirName), 0o755); err != nil {
		t.Fatalf("mkdir temp dir: %v", err)
	}

	initialized, err := hostWorkspaceInitialized(workspaceRoot)
	if err != nil {
		t.Fatalf("hostWorkspaceInitialized returned error: %v", err)
	}
	if initialized {
		t.Fatalf("expected workspace to be treated as empty")
	}

	if err := os.WriteFile(filepath.Join(workspaceRoot, "README.md"), []byte("hello\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	initialized, err = hostWorkspaceInitialized(workspaceRoot)
	if err != nil {
		t.Fatalf("hostWorkspaceInitialized returned error after file write: %v", err)
	}
	if !initialized {
		t.Fatalf("expected workspace to be treated as initialized")
	}
}

func TestGitCloneArgsUsesDepthOne(t *testing.T) {
	got := gitCloneArgs("https://example.test/repo.git", gitWorkspaceConfig{Branch: "main"}, "/tmp/workspace")
	want := []string{"clone", "--depth", "1", "--branch", "main", "https://example.test/repo.git", "/tmp/workspace"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("gitCloneArgs = %#v, want %#v", got, want)
	}
}

func TestGitCommitFetchArgs(t *testing.T) {
	got := gitCommitFetchArgs("e413509")
	want := []string{"fetch", "--depth", "1", "origin", "e413509"}
	if strings.Join(got, "\x00") != strings.Join(want, "\x00") {
		t.Fatalf("gitCommitFetchArgs = %#v, want %#v", got, want)
	}
}

func TestGitDeepenFetchArgs(t *testing.T) {
	gotUnshallow := gitDeepenFetchArgs(true)
	wantUnshallow := []string{"fetch", "--unshallow", "--tags", "origin", "+refs/heads/*:refs/remotes/origin/*"}
	if strings.Join(gotUnshallow, "\x00") != strings.Join(wantUnshallow, "\x00") {
		t.Fatalf("gitDeepenFetchArgs(true) = %#v, want %#v", gotUnshallow, wantUnshallow)
	}
	gotFull := gitDeepenFetchArgs(false)
	wantFull := []string{"fetch", "--tags", "origin", "+refs/heads/*:refs/remotes/origin/*"}
	if strings.Join(gotFull, "\x00") != strings.Join(wantFull, "\x00") {
		t.Fatalf("gitDeepenFetchArgs(false) = %#v, want %#v", gotFull, wantFull)
	}
}

func TestPrepareGitWorkspaceChecksOutPinnedCommit(t *testing.T) {
	t.Run("short SHA on the cloned branch via shallow file:// remote", func(t *testing.T) {
		// file:// forces a real shallow clone (a plain local path ignores --depth),
		// and its upload-pack rejects by-SHA fetches, exercising the deepen fallback.
		remote := "file://" + createLocalGitWorkspaceRepoWithHistory(t)
		workspacePath := runGitCommitWorkspace(t, remote, gitShortSHA(t, remote, "HEAD~1"))
		// HEAD~1 wrote "v1\n"; the branch tip rewrote it to "v2\n".
		assertFileContent(t, filepath.Join(workspacePath, "README.md"), "v1\n")
	})

	t.Run("short SHA on a different branch via deepen fallback", func(t *testing.T) {
		// The clone tracks the default branch only; the deepen fallback's
		// +refs/heads/* refspec must pull the feature branch so its commit resolves.
		remote := "file://" + createLocalGitWorkspaceRepoWithFeatureBranch(t)
		workspacePath := runGitCommitWorkspace(t, remote, gitShortSHA(t, remote, "feature"))
		assertFileContent(t, filepath.Join(workspacePath, "feat.txt"), "feat\n")
	})

	t.Run("short SHA against a non-shallow clone retries without --unshallow", func(t *testing.T) {
		// A plain local path makes git ignore --depth and produce a complete clone,
		// so the fallback's --unshallow fetch errors and must retry without it.
		remote := createLocalGitWorkspaceRepoWithHistory(t)
		workspacePath := runGitCommitWorkspace(t, remote, gitShortSHA(t, remote, "HEAD~1"))
		assertFileContent(t, filepath.Join(workspacePath, "README.md"), "v1\n")
	})
}

func runGitCommitWorkspace(t *testing.T, remote, commit string) string {
	t.Helper()
	session := &Session{Summary: SessionSummary{ID: "session-git-commit", WorkspacePath: filepath.Join(t.TempDir(), "workspace")}}
	workspace := WorkspaceConfig{
		ID:         "git-commit",
		Name:       "Git Commit",
		Type:       "git",
		ConfigJSON: fmt.Sprintf(`{"url":%q,"commit":%q}`, remote, commit),
	}
	if err := prepareGitWorkspace(context.Background(), session, workspace); err != nil {
		t.Fatalf("prepareGitWorkspace with commit %q returned error: %v", commit, err)
	}
	return session.Summary.WorkspacePath
}

func TestPrepareFileWorkspaceCopiesContent(t *testing.T) {
	testPrepareFileWorkspaceCopiesContent(t)
}

func testPrepareFileWorkspaceCopiesContent(t *testing.T) {
	t.Helper()
	config := &appconfig.Config{DataRoot: t.TempDir()}
	contentRoot := filepath.Join(config.DataRoot, "workspaces", "ws-file", fileWorkspaceContentDirName)
	if err := os.MkdirAll(contentRoot, 0o755); err != nil {
		t.Fatalf("mkdir content root: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contentRoot, "README.md"), []byte("workspace\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(contentRoot, "docs"), 0o755); err != nil {
		t.Fatalf("mkdir docs: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contentRoot, "docs", "guide.md"), []byte("guide\n"), 0o644); err != nil {
		t.Fatalf("write guide.md: %v", err)
	}
	session := &Session{Summary: SessionSummary{ID: "session-1", WorkspacePath: t.TempDir()}}
	workspace := WorkspaceConfig{
		ID:         "ws-file",
		Name:       "File Workspace",
		Type:       "file",
		ConfigJSON: encodeFileWorkspaceConfigForTest(t, contentRoot),
	}
	if err := prepareFileWorkspace(config, session, workspace); err != nil {
		t.Fatalf("prepareFileWorkspace returned error: %v", err)
	}
	assertFileContent(t, filepath.Join(session.Summary.WorkspacePath, "README.md"), "workspace\n")
	assertFileContent(t, filepath.Join(session.Summary.WorkspacePath, "docs", "guide.md"), "guide\n")
	if err := os.WriteFile(filepath.Join(contentRoot, "README.md"), []byte("updated\n"), 0o644); err != nil {
		t.Fatalf("overwrite README.md: %v", err)
	}
	if err := prepareFileWorkspace(config, session, workspace); err != nil {
		t.Fatalf("prepareFileWorkspace on refresh returned error: %v", err)
	}
	assertFileContent(t, filepath.Join(session.Summary.WorkspacePath, "README.md"), "updated\n")
}

func TestPrepareSessionWorkspacePrefersSessionSnapshot(t *testing.T) {
	config := &appconfig.Config{DataRoot: t.TempDir()}
	workspaceID := "snapshot-file"
	contentRoot := mustDefaultFileWorkspaceContentRoot(t, config, workspaceID)
	if err := os.MkdirAll(contentRoot, 0o755); err != nil {
		t.Fatalf("MkdirAll content root returned error: %v", err)
	}
	if err := os.WriteFile(filepath.Join(contentRoot, "snapshot.txt"), []byte("snapshot\n"), 0o644); err != nil {
		t.Fatalf("WriteFile snapshot returned error: %v", err)
	}
	session := &Session{
		Summary:     SessionSummary{ID: "session-snapshot", WorkspacePath: filepath.Join(t.TempDir(), "workspace")},
		WorkspaceID: workspaceID,
		Workspace: &SessionWorkspace{
			ID:         workspaceID,
			Name:       "Snapshot File Workspace",
			Type:       "file",
			ConfigJSON: defaultFileWorkspaceConfigJSON(config, workspaceID),
		},
	}
	if err := prepareSessionWorkspace(context.Background(), config, nil, session); err != nil {
		t.Fatalf("prepareSessionWorkspace returned error: %v", err)
	}
	assertFileContent(t, filepath.Join(session.Summary.WorkspacePath, "snapshot.txt"), "snapshot\n")
}

func TestFileWorkspaceContentRootRejectsOutsideDataRoot(t *testing.T) {
	config := &appconfig.Config{DataRoot: t.TempDir()}
	workspace := WorkspaceConfig{
		ID:         "ws-file",
		Name:       "File Workspace",
		Type:       "file",
		ConfigJSON: encodeFileWorkspaceConfigForTest(t, t.TempDir()),
	}
	if _, err := fileWorkspaceContentRoot(config, workspace); err == nil {
		t.Fatalf("expected outside data root to be rejected")
	}
}

func TestExtractWorkspaceTarArchiveRejectsSymlinkEscape(t *testing.T) {
	contentRoot := t.TempDir()
	root, err := os.OpenRoot(contentRoot)
	if err != nil {
		t.Fatalf("OpenRoot contentRoot: %v", err)
	}
	defer func() { _ = root.Close() }()
	outsideRoot := t.TempDir()
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	if err := tw.WriteHeader(&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: outsideRoot, Mode: 0o777}); err != nil {
		t.Fatalf("WriteHeader symlink: %v", err)
	}
	body := "escape\n"
	if err := tw.WriteHeader(&tar.Header{Name: "link/owned.txt", Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatalf("WriteHeader escaped file: %v", err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatalf("write escaped body: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := extractWorkspaceTarArchive(&archive, root); err == nil {
		t.Fatalf("expected symlink tar entry to be rejected")
	}
	if _, err := os.Stat(filepath.Join(outsideRoot, "owned.txt")); !os.IsNotExist(err) {
		t.Fatalf("expected outside file to be absent, stat err=%v", err)
	}
}

func TestExtractWorkspaceTarArchiveDirectoryEntryAfterFileKeepsContent(t *testing.T) {
	contentRoot := t.TempDir()
	root, err := os.OpenRoot(contentRoot)
	if err != nil {
		t.Fatalf("OpenRoot contentRoot: %v", err)
	}
	defer func() { _ = root.Close() }()
	var archive bytes.Buffer
	tw := tar.NewWriter(&archive)
	body := "content\n"
	if err := tw.WriteHeader(&tar.Header{Name: "dir/file.txt", Mode: 0o644, Size: int64(len(body))}); err != nil {
		t.Fatalf("WriteHeader file: %v", err)
	}
	if _, err := tw.Write([]byte(body)); err != nil {
		t.Fatalf("write file body: %v", err)
	}
	if err := tw.WriteHeader(&tar.Header{Name: "dir", Typeflag: tar.TypeDir, Mode: 0o755}); err != nil {
		t.Fatalf("WriteHeader dir: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("close tar writer: %v", err)
	}
	if err := extractWorkspaceTarArchive(&archive, root); err != nil {
		t.Fatalf("extractWorkspaceTarArchive returned error: %v", err)
	}
	assertFileContent(t, filepath.Join(contentRoot, "dir", "file.txt"), body)
}

func TestPrepareGitWorkspaceClonesRootAndTarget(t *testing.T) {
	testPrepareGitWorkspaceClonesRootAndTarget(t)
}

func testPrepareGitWorkspaceClonesRootAndTarget(t *testing.T) {
	t.Helper()
	ctx := context.Background()
	remote := createLocalGitWorkspaceRepo(t)

	rootSession := &Session{Summary: SessionSummary{ID: "session-git-root", WorkspacePath: filepath.Join(t.TempDir(), "workspace")}}
	rootWorkspace := WorkspaceConfig{
		ID:         "git-root",
		Name:       "Git Root",
		Type:       "git",
		ConfigJSON: fmt.Sprintf(`{"url":%q}`, remote),
	}
	if err := prepareGitWorkspace(ctx, rootSession, rootWorkspace); err != nil {
		t.Fatalf("prepareGitWorkspace root returned error: %v", err)
	}
	assertFileContent(t, filepath.Join(rootSession.Summary.WorkspacePath, "README.md"), "root\n")
	assertFileContent(t, filepath.Join(rootSession.Summary.WorkspacePath, "nested", "data.txt"), "nested\n")
	if _, err := os.Stat(filepath.Join(rootSession.Summary.WorkspacePath, gitWorkspaceTempDirName)); !os.IsNotExist(err) {
		t.Fatalf("expected temp git clone dir to be removed, stat err=%v", err)
	}
	if err := os.WriteFile(filepath.Join(rootSession.Summary.WorkspacePath, "local.txt"), []byte("local\n"), 0o644); err != nil {
		t.Fatalf("write local workspace file: %v", err)
	}
	if err := prepareGitWorkspace(ctx, rootSession, rootWorkspace); err != nil {
		t.Fatalf("prepareGitWorkspace initialized root returned error: %v", err)
	}
	assertFileContent(t, filepath.Join(rootSession.Summary.WorkspacePath, "local.txt"), "local\n")

	targetSession := &Session{Summary: SessionSummary{ID: "session-git-target", WorkspacePath: filepath.Join(t.TempDir(), "workspace")}}
	targetWorkspace := WorkspaceConfig{
		ID:         "git-target",
		Name:       "Git Target",
		Type:       "git",
		ConfigJSON: fmt.Sprintf(`{"url":%q,"path":"vendor/repo"}`, remote),
	}
	if err := prepareGitWorkspace(ctx, targetSession, targetWorkspace); err != nil {
		t.Fatalf("prepareGitWorkspace target returned error: %v", err)
	}
	assertFileContent(t, filepath.Join(targetSession.Summary.WorkspacePath, "vendor", "repo", "README.md"), "root\n")

	if got := applyGitCredentials("https://example.test/repo.git", gitWorkspaceConfig{Username: "user name", Password: "p@ss"}); got != "https://user+name:p%40ss@example.test/repo.git" {
		t.Fatalf("applyGitCredentials username/password = %q", got)
	}
	if got := applyGitCredentials("https://example.test/repo.git", gitWorkspaceConfig{Credential: "token"}); got != "https://token@example.test/repo.git" {
		t.Fatalf("applyGitCredentials token = %q", got)
	}
	if got := applyGitCredentials("ssh://example.test/repo.git", gitWorkspaceConfig{Credential: "token"}); got != "ssh://example.test/repo.git" {
		t.Fatalf("applyGitCredentials ssh = %q", got)
	}
}

func createLocalGitWorkspaceRepo(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "remote")
	if err := os.MkdirAll(filepath.Join(repo, "nested"), 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	assertGitCommand(t, repo, "init", ".")
	assertGitCommand(t, repo, "config", "user.email", "agent-compose@example.test")
	assertGitCommand(t, repo, "config", "user.name", "agent-compose")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("root\n"), 0o644); err != nil {
		t.Fatalf("write README.md: %v", err)
	}
	if err := os.WriteFile(filepath.Join(repo, "nested", "data.txt"), []byte("nested\n"), 0o644); err != nil {
		t.Fatalf("write nested data: %v", err)
	}
	assertGitCommand(t, repo, "add", ".")
	assertGitCommand(t, repo, "commit", "-m", "initial")
	return repo
}

func createLocalGitWorkspaceRepoWithHistory(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "remote")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	assertGitCommand(t, repo, "init", ".")
	assertGitCommand(t, repo, "config", "user.email", "agent-compose@example.test")
	assertGitCommand(t, repo, "config", "user.name", "agent-compose")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("write README v1: %v", err)
	}
	assertGitCommand(t, repo, "add", ".")
	assertGitCommand(t, repo, "commit", "-m", "v1")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("write README v2: %v", err)
	}
	assertGitCommand(t, repo, "add", ".")
	assertGitCommand(t, repo, "commit", "-m", "v2")
	return repo
}

func createLocalGitWorkspaceRepoWithFeatureBranch(t *testing.T) string {
	t.Helper()
	repo := filepath.Join(t.TempDir(), "remote")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	assertGitCommand(t, repo, "init", "-b", "main", ".")
	assertGitCommand(t, repo, "config", "user.email", "agent-compose@example.test")
	assertGitCommand(t, repo, "config", "user.name", "agent-compose")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("v1\n"), 0o644); err != nil {
		t.Fatalf("write README v1: %v", err)
	}
	assertGitCommand(t, repo, "add", ".")
	assertGitCommand(t, repo, "commit", "-m", "v1")
	// feature branches off v1 and adds a file that exists nowhere on main.
	assertGitCommand(t, repo, "checkout", "-b", "feature")
	if err := os.WriteFile(filepath.Join(repo, "feat.txt"), []byte("feat\n"), 0o644); err != nil {
		t.Fatalf("write feat.txt: %v", err)
	}
	assertGitCommand(t, repo, "add", ".")
	assertGitCommand(t, repo, "commit", "-m", "feat")
	// Advance main past the branch point so the feature commit is not reachable
	// from the cloned default branch.
	assertGitCommand(t, repo, "checkout", "main")
	if err := os.WriteFile(filepath.Join(repo, "README.md"), []byte("v2\n"), 0o644); err != nil {
		t.Fatalf("write README v2: %v", err)
	}
	assertGitCommand(t, repo, "add", ".")
	assertGitCommand(t, repo, "commit", "-m", "v2")
	return repo
}

func gitShortSHA(t *testing.T, remote, rev string) string {
	t.Helper()
	dir := strings.TrimPrefix(remote, "file://")
	cmd := exec.Command("git", "rev-parse", "--short", rev)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git rev-parse %s failed: %v\n%s", rev, err, string(output))
	}
	return strings.TrimSpace(string(output))
}

func assertGitCommand(t *testing.T, dir string, args ...string) {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("git %v failed: %v\n%s", args, err, string(output))
	}
}

func encodeFileWorkspaceConfigForTest(t *testing.T, root string) string {
	t.Helper()
	payload, err := json.Marshal(fileWorkspaceConfig{Root: root})
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return string(payload)
}

func defaultFileWorkspaceConfigJSON(config *appconfig.Config, workspaceID string) string {
	root, err := defaultFileWorkspaceContentRoot(config, workspaceID)
	if err != nil {
		return "{}"
	}
	payload, err := json.Marshal(fileWorkspaceConfig{Root: root})
	if err != nil {
		return "{}"
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

func assertFileContent(t *testing.T, path, want string) {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile %s: %v", path, err)
	}
	if string(data) != want {
		t.Fatalf("file %s = %q, want %q", path, string(data), want)
	}
}
