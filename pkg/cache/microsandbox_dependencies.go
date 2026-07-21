package cache

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type CombinedMaterializedDependencies []MaterializedDependencyProvider

func (providers CombinedMaterializedDependencies) MaterializedDependencies(ctx context.Context) ([]MaterializedDependency, []string, error) {
	var dependencies []MaterializedDependency
	var warnings []string
	for _, provider := range providers {
		if provider == nil {
			continue
		}
		items, providerWarnings, err := provider.MaterializedDependencies(ctx)
		warnings = AppendWarnings(warnings, providerWarnings...)
		if err != nil {
			return nil, warnings, err
		}
		dependencies = append(dependencies, items...)
	}
	return dependencies, warnings, nil
}

type MicrosandboxRootfsDependencies struct {
	Home string
}

func (p MicrosandboxRootfsDependencies) MaterializedDependencies(ctx context.Context) ([]MaterializedDependency, []string, error) {
	home := strings.TrimSpace(p.Home)
	if home == "" {
		return nil, nil, nil
	}
	root := filepath.Join(home, "rootfs-disks")
	rootInfo, err := os.Lstat(root)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil, nil
		}
		return nil, nil, fmt.Errorf("inspect microsandbox rootfs dependencies %s: %w", root, err)
	}
	if rootInfo.Mode()&os.ModeSymlink != 0 || !rootInfo.IsDir() {
		return nil, nil, fmt.Errorf("microsandbox rootfs dependencies %s is not a regular directory", root)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, nil, fmt.Errorf("read microsandbox rootfs dependencies %s: %w", root, err)
	}
	var dependencies []MaterializedDependency
	var warnings []string
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return nil, warnings, err
		}
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".qcow2.owner.json") {
			continue
		}
		path := filepath.Join(root, entry.Name())
		info, err := os.Lstat(path)
		if err != nil {
			return nil, warnings, fmt.Errorf("inspect microsandbox rootfs ownership %s: %w", path, err)
		}
		if info.Mode()&os.ModeSymlink != 0 || !info.Mode().IsRegular() {
			return nil, warnings, fmt.Errorf("microsandbox rootfs ownership %s is not a regular file", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, warnings, fmt.Errorf("read microsandbox rootfs ownership %s: %w", path, err)
		}
		var sidecar struct {
			Version      int    `json:"version"`
			ResourceKind string `json:"resource_kind"`
			SandboxID    string `json:"sandbox_id"`
			BaseIdentity string `json:"base_cache_identity"`
			BackingPath  string `json:"backing_file_path"`
		}
		if err := json.Unmarshal(data, &sidecar); err != nil || sidecar.Version != 2 || sidecar.ResourceKind != "microsandbox-rootfs" || strings.TrimSpace(sidecar.SandboxID) == "" || strings.TrimSpace(sidecar.BaseIdentity) == "" || !filepath.IsAbs(sidecar.BackingPath) {
			return nil, warnings, fmt.Errorf("microsandbox rootfs ownership %s cannot safely identify its base disk", path)
		}
		dependencies = append(dependencies, MaterializedDependency{SandboxID: sidecar.SandboxID, Identity: sidecar.BaseIdentity, Path: filepath.Clean(sidecar.BackingPath), Status: "owned"})
	}
	return dependencies, warnings, nil
}
