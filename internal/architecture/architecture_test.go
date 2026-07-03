package architecture_test

import (
	"encoding/json"
	"io"
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
	for _, pkg := range listGoPackages(t, root, "./pkg/...") {
		for _, imported := range pkg.Imports {
			if strings.HasPrefix(imported, module+"/internal/") {
				t.Fatalf("%s imports internal package %s", pkg.ImportPath, imported)
			}
		}
	}
}

func TestTransportPackagesDoNotAddAppImports(t *testing.T) {
	root := repoRoot(t)
	module := strings.TrimSpace(runCommand(t, root, "go", "list", "-m"))

	checkNoDisallowedImports(t, root, []string{"./internal/transport/..."}, []importRule{
		{path: module + "/internal/app"},
	}, nil)
}

func TestDomainPackagesDoNotImportHandlerFrameworks(t *testing.T) {
	root := repoRoot(t)
	module := strings.TrimSpace(runCommand(t, root, "go", "list", "-m"))
	var domainPkgs []goPackage
	for _, pkg := range listGoPackages(t, root, "./internal/...") {
		if pkg.ImportPath == module+"/internal/app" ||
			strings.HasPrefix(pkg.ImportPath, module+"/internal/app/") ||
			strings.HasPrefix(pkg.ImportPath, module+"/internal/persistence/") ||
			strings.HasPrefix(pkg.ImportPath, module+"/internal/transport/") ||
			strings.HasPrefix(pkg.ImportPath, module+"/internal/architecture") {
			continue
		}
		domainPkgs = append(domainPkgs, pkg)
	}

	checkPackagesDoNotImport(t, domainPkgs, []importRule{
		{path: "connectrpc.com/connect"},
		{path: "github.com/labstack/echo/v4"},
		{path: module + "/internal/transport/", prefix: true},
	}, nil)
	checkPackagesDoNotImportGeneratedConnect(t, domainPkgs, module, nil)
}

func TestPersistencePackagesDoNotImportTransportFrameworks(t *testing.T) {
	root := repoRoot(t)
	module := strings.TrimSpace(runCommand(t, root, "go", "list", "-m"))
	checkNoDisallowedImports(t, root, []string{"./internal/persistence/..."}, []importRule{
		{path: "connectrpc.com/connect"},
		{path: "github.com/labstack/echo/v4"},
		{path: module + "/internal/transport/", prefix: true},
	}, nil)
}

func TestGeneratedConnectPackagesStayInRouteAdapters(t *testing.T) {
	root := repoRoot(t)
	module := strings.TrimSpace(runCommand(t, root, "go", "list", "-m"))
	allow := map[string]bool{
		module + "/internal/app":       true,
		module + "/cmd/agent-compose":  true,
		module + "/pkg/health":         true,
		module + "/internal/transport": true,
	}

	for _, pkg := range listGoPackages(t, root, "./...") {
		for _, imported := range pkg.Imports {
			if !isGeneratedConnectPackage(imported, module) {
				continue
			}
			if allow[pkg.ImportPath] || strings.HasPrefix(pkg.ImportPath, module+"/internal/transport/") {
				continue
			}
			t.Fatalf("%s imports generated Connect handler package %s", pkg.ImportPath, imported)
		}
	}
}

func TestProjectPackageDoesNotImportTransportHandlers(t *testing.T) {
	root := repoRoot(t)
	projectRoot := filepath.Join(root, "internal", "project")
	if _, err := os.Stat(projectRoot); os.IsNotExist(err) {
		return
	} else if err != nil {
		t.Fatalf("stat %s: %v", projectRoot, err)
	}

	module := strings.TrimSpace(runCommand(t, root, "go", "list", "-m"))
	projectPkgs := listGoPackages(t, root, "./internal/project/...")
	checkPackagesDoNotImport(t, projectPkgs, []importRule{
		{path: "connectrpc.com/connect"},
		{path: "github.com/labstack/echo/v4"},
	}, nil)
	checkPackagesDoNotImportGeneratedConnect(t, projectPkgs, module, nil)
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

type importRule struct {
	path   string
	prefix bool
}

func checkNoDisallowedImports(t *testing.T, root string, patterns []string, rules []importRule, allow map[string]map[string]bool) {
	t.Helper()
	checkPackagesDoNotImport(t, listGoPackages(t, root, patterns...), rules, allow)
}

func checkPackagesDoNotImport(t *testing.T, packages []goPackage, rules []importRule, allow map[string]map[string]bool) {
	t.Helper()
	for _, pkg := range packages {
		for _, imported := range pkg.Imports {
			if !matchesAnyImportRule(imported, rules) {
				continue
			}
			if allow[pkg.ImportPath][imported] {
				continue
			}
			t.Fatalf("%s imports disallowed package %s", pkg.ImportPath, imported)
		}
	}
}

func checkPackagesDoNotImportGeneratedConnect(t *testing.T, packages []goPackage, module string, allow map[string]map[string]bool) {
	t.Helper()
	for _, pkg := range packages {
		for _, imported := range pkg.Imports {
			if !isGeneratedConnectPackage(imported, module) {
				continue
			}
			if allow[pkg.ImportPath][imported] {
				continue
			}
			t.Fatalf("%s imports generated Connect handler package %s", pkg.ImportPath, imported)
		}
	}
}

func isGeneratedConnectPackage(imported, module string) bool {
	return strings.HasPrefix(imported, module+"/proto/") && strings.HasSuffix(imported, "connect")
}

func matchesAnyImportRule(imported string, rules []importRule) bool {
	for _, rule := range rules {
		if rule.prefix {
			if strings.HasPrefix(imported, rule.path) {
				return true
			}
			continue
		}
		if imported == rule.path {
			return true
		}
	}
	return false
}

func listGoPackages(t *testing.T, root string, patterns ...string) []goPackage {
	t.Helper()
	args := append([]string{"list", "-json"}, patterns...)
	output := runCommand(t, root, "go", args...)
	decoder := json.NewDecoder(strings.NewReader(output))
	var packages []goPackage
	for {
		var pkg goPackage
		err := decoder.Decode(&pkg)
		if err == io.EOF {
			return packages
		}
		if err != nil {
			t.Fatalf("decode go list output: %v", err)
		}
		packages = append(packages, pkg)
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
