package main

import (
	"encoding/json"
	"net/http/httptest"
	"os"
	"testing"
)

func TestE2ECLIVolumeLifecycleWithInProcessDaemon(t *testing.T) {
	app, cancel := newTestDaemonApp(t, "127.0.0.1:0", nil)
	defer cancel()
	server := httptest.NewServer(app.Echo)
	defer server.Close()

	cache := createVolumeThroughCLI(t, server.URL, "cache")
	state := createVolumeThroughCLI(t, server.URL, "state")
	for _, volume := range []composeVolumeOutput{cache, state} {
		if volume.Driver != "local" || volume.Path == "" {
			t.Fatalf("created volume = %#v", volume)
		}
		if info, err := os.Stat(volume.Path); err != nil || !info.IsDir() {
			t.Fatalf("created volume path %q info=%#v err=%v", volume.Path, info, err)
		}
	}

	listOut, listErr, _, listCode := executeCLICommand("volume", "ls", "--host", server.URL, "--json", "--driver", "local")
	if listCode != 0 || listErr != "" {
		t.Fatalf("volume ls code/stderr = %d / %q", listCode, listErr)
	}
	var listed composeVolumeListOutput
	if err := json.Unmarshal([]byte(listOut), &listed); err != nil {
		t.Fatalf("decode volume list: %v\n%s", err, listOut)
	}
	if len(listed.Volumes) != 2 {
		t.Fatalf("listed volumes = %#v", listed.Volumes)
	}

	inspectOut, inspectErr, _, inspectCode := executeCLICommand("inspect", "volume", "--host", server.URL, "--json", "cache")
	if inspectCode != 0 || inspectErr != "" {
		t.Fatalf("volume inspect code/stderr = %d / %q", inspectCode, inspectErr)
	}
	var inspected composeVolumeInspectOutput
	if err := json.Unmarshal([]byte(inspectOut), &inspected); err != nil {
		t.Fatalf("decode volume inspect: %v\n%s", err, inspectOut)
	}
	if inspected.Volume.Name != "cache" || inspected.Volume.Path != cache.Path {
		t.Fatalf("inspected volume = %#v, want path %q", inspected.Volume, cache.Path)
	}

	dryOut, dryErr, _, dryCode := executeCLICommand("volume", "prune", "--host", server.URL, "--json", "--driver", "local")
	if dryCode != 0 || dryErr != "" {
		t.Fatalf("volume prune dry-run code/stderr = %d / %q", dryCode, dryErr)
	}
	var dryRun composeVolumePruneOutput
	if err := json.Unmarshal([]byte(dryOut), &dryRun); err != nil {
		t.Fatalf("decode volume prune dry-run: %v\n%s", err, dryOut)
	}
	if !dryRun.DryRun || len(dryRun.Matched) != 2 || len(dryRun.Removed) != 0 {
		t.Fatalf("volume prune dry-run = %#v", dryRun)
	}

	removeOut, removeErr, _, removeCode := executeCLICommand("volume", "rm", "--host", server.URL, "--json", "--force", "cache")
	if removeCode != 0 || removeErr != "" {
		t.Fatalf("volume rm code/stderr = %d / %q", removeCode, removeErr)
	}
	var removed composeVolumeRemoveOutput
	if err := json.Unmarshal([]byte(removeOut), &removed); err != nil {
		t.Fatalf("decode volume removal: %v\n%s", err, removeOut)
	}
	if len(removed.Removed) != 1 || removed.Removed[0] != "cache" {
		t.Fatalf("removed volumes = %#v", removed.Removed)
	}
	if _, err := os.Stat(cache.Path); !os.IsNotExist(err) {
		t.Fatalf("removed cache path still exists: %v", err)
	}

	pruneOut, pruneErr, _, pruneCode := executeCLICommand("volume", "prune", "--host", server.URL, "--json", "--driver", "local", "--force")
	if pruneCode != 0 || pruneErr != "" {
		t.Fatalf("volume prune code/stderr = %d / %q", pruneCode, pruneErr)
	}
	var pruned composeVolumePruneOutput
	if err := json.Unmarshal([]byte(pruneOut), &pruned); err != nil {
		t.Fatalf("decode forced volume prune: %v\n%s", err, pruneOut)
	}
	if pruned.DryRun || len(pruned.Removed) != 1 || pruned.Removed[0].Name != "state" {
		t.Fatalf("forced volume prune = %#v", pruned)
	}
	if _, err := os.Stat(state.Path); !os.IsNotExist(err) {
		t.Fatalf("pruned state path still exists: %v", err)
	}
}

func createVolumeThroughCLI(t *testing.T, host, name string) composeVolumeOutput {
	t.Helper()
	stdout, stderr, _, exitCode := executeCLICommand("volume", "create", "--host", host, "--json", "--label", "purpose="+name, name)
	if exitCode != 0 || stderr != "" {
		t.Fatalf("volume create %s code/stderr = %d / %q", name, exitCode, stderr)
	}
	var created composeVolumeCreateOutput
	if err := json.Unmarshal([]byte(stdout), &created); err != nil {
		t.Fatalf("decode volume create %s: %v\n%s", name, err, stdout)
	}
	if !created.Created || created.Volume.Name != name || created.Volume.Labels["purpose"] != name {
		t.Fatalf("created volume %s = %#v", name, created)
	}
	return created.Volume
}
