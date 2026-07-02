package architecture_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

type goPackage struct {
	ImportPath string
	Imports    []string
}

func TestPkgPackagesDoNotImportInternalPackages(t *testing.T) {
	root := repoRoot(t)
	module := strings.TrimSpace(runCommand(t, root, "go", "list", "-m"))
	output := runCommand(t, root, "go", "list", "-json", "./pkg/...")
	decoder := json.NewDecoder(strings.NewReader(output))
	for decoder.More() {
		var pkg goPackage
		if err := decoder.Decode(&pkg); err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		for _, imported := range pkg.Imports {
			if strings.HasPrefix(imported, module+"/internal/") {
				t.Fatalf("%s imports internal package %s", pkg.ImportPath, imported)
			}
		}
	}
}

func TestRemovedPackageReferencesDoNotReturn(t *testing.T) {
	root := repoRoot(t)
	files := []string{
		"README.md",
		"AGENTS.md",
		"runtime/javascript/src/system-context.ts",
	}
	files = append(files, markdownFiles(t, filepath.Join(root, "docs"))...)
	for _, file := range files {
		content, err := os.ReadFile(filepath.Join(root, file))
		if err != nil {
			t.Fatalf("read %s: %v", file, err)
		}
		text := string(content)
		for _, disallowed := range []string{
			"pkg/" + "agentcompose",
			"agentcompose" + ".Setup",
			"agentcompose" + ".Register",
			"agentcompose" + ".StartBackground",
		} {
			if strings.Contains(text, disallowed) {
				t.Fatalf("%s still references removed package marker %q", file, disallowed)
			}
		}
	}
}

func TestRefactorMigrationArtifactsDoNotReturn(t *testing.T) {
	root := repoRoot(t)
	for _, dir := range []string{"internal", "pkg", "scripts"} {
		absRoot := filepath.Join(root, dir)
		if _, err := os.Stat(absRoot); os.IsNotExist(err) {
			continue
		}
		err := filepath.WalkDir(absRoot, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() {
				return nil
			}
			name := entry.Name()
			if strings.Contains(name, "_refactor") || strings.HasPrefix(name, "refactor_") {
				t.Fatalf("migration artifact file remains: %s", relativePath(root, path))
			}
			return nil
		})
		if err != nil {
			t.Fatalf("walk %s: %v", dir, err)
		}
	}
}

func markdownFiles(t *testing.T, root string) []string {
	t.Helper()
	var files []string
	err := filepath.WalkDir(root, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() {
			return nil
		}
		if strings.EqualFold(filepath.Ext(path), ".md") {
			files = append(files, relativePath(repoRoot(t), path))
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk %s: %v", root, err)
	}
	return files
}

func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("get working directory: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not find repository root")
		}
		dir = parent
	}
}

func relativePath(root, path string) string {
	rel, err := filepath.Rel(root, path)
	if err != nil {
		return path
	}
	return rel
}

func runCommand(t *testing.T, dir, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	output, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("%s %s failed: %v\n%s", name, strings.Join(args, " "), err, output)
	}
	return string(output)
}
