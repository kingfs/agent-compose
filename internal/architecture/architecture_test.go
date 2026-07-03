package architecture_test

import (
	"encoding/json"
	"go/ast"
	"go/parser"
	"go/printer"
	"go/token"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
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

func TestAppFacadeMayUseProjectFoundation(t *testing.T) {
	root := repoRoot(t)
	projectRoot := filepath.Join(root, "internal", "project")
	if _, err := os.Stat(projectRoot); os.IsNotExist(err) {
		t.Skip("internal/project does not exist yet")
	} else if err != nil {
		t.Fatalf("stat %s: %v", projectRoot, err)
	}

	module := strings.TrimSpace(runCommand(t, root, "go", "list", "-m"))
	projectImport := module + "/internal/project"
	for _, pkg := range listGoPackages(t, root, "./internal/app") {
		for _, imported := range pkg.Imports {
			if imported == projectImport || strings.HasPrefix(imported, projectImport+"/") {
				return
			}
		}
	}
	t.Fatalf("internal/app must use internal/project foundation types during ProjectService migration")
}

func TestProjectPackageHasUsecaseFoundationShape(t *testing.T) {
	root := repoRoot(t)
	projectRoot := filepath.Join(root, "internal", "project")
	entries, err := os.ReadDir(projectRoot)
	if err != nil {
		t.Fatalf("read %s: %v", projectRoot, err)
	}

	var nonTestGoFiles []string
	combined := strings.Builder{}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		nonTestGoFiles = append(nonTestGoFiles, name)
		content, err := os.ReadFile(filepath.Join(projectRoot, name))
		if err != nil {
			t.Fatalf("read internal/project/%s: %v", name, err)
		}
		combined.Write(content)
		combined.WriteByte('\n')
	}

	if len(nonTestGoFiles) < 3 {
		t.Fatalf("internal/project has %d non-test Go files (%s), want at least foundation plus usecase shape", len(nonTestGoFiles), strings.Join(nonTestGoFiles, ", "))
	}
	text := combined.String()
	for _, marker := range []string{
		"Package project contains transport-agnostic Project usecase",
		"type ErrorKind",
		"type Error struct",
		"type ApplyResult",
		"type ValidationIssue",
	} {
		if !strings.Contains(text, marker) {
			t.Fatalf("internal/project is missing foundation/usecase marker %q", marker)
		}
	}
}

func TestProjectPackageDoesNotImportAppOrTransportHandlers(t *testing.T) {
	root := repoRoot(t)
	projectRoot := filepath.Join(root, "internal", "project")
	if _, err := os.Stat(projectRoot); os.IsNotExist(err) {
		t.Skip("internal/project does not exist yet")
	} else if err != nil {
		t.Fatalf("stat %s: %v", projectRoot, err)
	}

	module := strings.TrimSpace(runCommand(t, root, "go", "list", "-m"))
	projectPkgs := listGoPackages(t, root, "./internal/project/...")
	checkPackagesDoNotImport(t, projectPkgs, []importRule{
		{path: module + "/internal/app"},
		{path: module + "/internal/app/", prefix: true},
		{path: module + "/internal/transport/", prefix: true},
		{path: "connectrpc.com/connect"},
		{path: "github.com/labstack/echo/v4"},
	}, nil)
	checkPackagesDoNotImportGeneratedConnect(t, projectPkgs, module, nil)
}

func TestProjectPackageDoesNotImportPersistenceAdapters(t *testing.T) {
	root := repoRoot(t)
	projectRoot := filepath.Join(root, "internal", "project")
	if _, err := os.Stat(projectRoot); os.IsNotExist(err) {
		t.Skip("internal/project does not exist yet")
	} else if err != nil {
		t.Fatalf("stat %s: %v", projectRoot, err)
	}

	module := strings.TrimSpace(runCommand(t, root, "go", "list", "-m"))
	disallowed := []importRule{
		{path: module + "/internal/persistence/", prefix: true},
	}
	entries, err := os.ReadDir(projectRoot)
	if err != nil {
		t.Fatalf("read %s: %v", projectRoot, err)
	}
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if !strings.HasSuffix(name, ".go") || strings.HasSuffix(name, "_test.go") {
			continue
		}
		file := filepath.Join(projectRoot, name)
		for _, imported := range goFileImports(t, file) {
			if !matchesAnyImportRule(imported, disallowed) {
				continue
			}
			t.Fatalf("%s imports persistence adapter %s", relativePath(root, file), imported)
		}
	}
}

func TestProjectAppFilesDoNotRegressToMigrationArtifacts(t *testing.T) {
	root := repoRoot(t)
	files, err := filepath.Glob(filepath.Join(root, "internal", "app", "*project*.go"))
	if err != nil {
		t.Fatalf("glob project app files: %v", err)
	}
	files = append(files,
		filepath.Join(root, "internal", "app", "project_apply_service.go"),
		filepath.Join(root, "internal", "app", "project_facade.go"),
	)

	seen := map[string]bool{}
	for _, file := range files {
		if seen[file] || strings.HasSuffix(file, "_test.go") {
			continue
		}
		seen[file] = true
		content, err := os.ReadFile(file)
		if err != nil {
			t.Fatalf("read %s: %v", relativePath(root, file), err)
		}
		text := string(content)
		for _, disallowed := range []string{
			"ProjectMigration",
			"projectMigration",
			"refactorProject",
			"projectRefactor",
			"type projectApplyError struct",
			"type projectValidationError struct",
			"type applyProjectError struct",
			"type validationProjectError struct",
		} {
			if strings.Contains(text, disallowed) {
				t.Fatalf("%s reintroduced project migration artifact %q", relativePath(root, file), disallowed)
			}
		}
	}
}

func TestProjectFacadeGeneratedConnectOwnershipOnlyShrinks(t *testing.T) {
	root := repoRoot(t)
	facadeFile := filepath.Join(root, "internal", "app", "project_facade.go")
	methods := projectServiceMethods(t, facadeFile)
	allowed := map[string]bool{
		"ValidateProject": true,
		"ApplyProject":    true,
		"WatchProject":    true,
	}
	for method := range methods {
		if !allowed[method] {
			t.Fatalf("internal/app Project facade owns new generated Connect handler method %s; add protocol mapping in internal/transport/connect instead", method)
		}
	}
	if len(methods) > len(allowed) {
		t.Fatalf("internal/app Project facade owns %d generated Connect handler methods, want no more than %d", len(methods), len(allowed))
	}

	for _, method := range []string{"GetProject", "ListProjects", "RemoveProject"} {
		if methods[method] {
			t.Fatalf("internal/app Project facade still owns generated Connect handler signature for %s; query protocol mapping belongs in internal/transport/connect", method)
		}
	}
	for method, passthrough := range projectTransportQueryPassthroughHandlers(t, root) {
		if passthrough {
			t.Fatalf("internal/transport/connect still passes generated %s request directly to app Project facade", method)
		}
	}
}

func TestProjectFacadeDoesNotAddPrivateProjectErrorTypes(t *testing.T) {
	root := repoRoot(t)
	files := []string{
		filepath.Join(root, "internal", "app", "project_facade.go"),
		filepath.Join(root, "internal", "app", "project_apply_service.go"),
	}
	for _, file := range files {
		parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
		if err != nil {
			t.Fatalf("parse %s: %v", relativePath(root, file), err)
		}
		for _, decl := range parsed.Decls {
			gen, ok := decl.(*ast.GenDecl)
			if !ok || gen.Tok != token.TYPE {
				continue
			}
			for _, spec := range gen.Specs {
				typeSpec, ok := spec.(*ast.TypeSpec)
				if !ok {
					continue
				}
				name := typeSpec.Name.Name
				if ast.IsExported(name) || !strings.Contains(strings.ToLower(name), "project") || !strings.HasSuffix(name, "Error") {
					continue
				}
				t.Fatalf("%s defines app-local private Project error type %s; use internal/project error classification instead", relativePath(root, file), name)
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

func projectServiceMethods(t *testing.T, file string) map[string]bool {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, 0)
	if err != nil {
		t.Fatalf("parse %s: %v", file, err)
	}
	methods := map[string]bool{}
	for _, decl := range parsed.Decls {
		fn, ok := decl.(*ast.FuncDecl)
		if !ok || fn.Recv == nil || len(fn.Recv.List) == 0 {
			continue
		}
		if !isReceiverNamed(fn.Recv.List[0].Type, "ProjectService") {
			continue
		}
		if fn.Type.Params == nil || len(fn.Type.Params.List) == 0 || !fieldListContainsConnectRequest(fn.Type.Params) {
			continue
		}
		methods[fn.Name.Name] = true
	}
	return methods
}

func isReceiverNamed(expr ast.Expr, name string) bool {
	switch typed := expr.(type) {
	case *ast.Ident:
		return typed.Name == name
	case *ast.StarExpr:
		return isReceiverNamed(typed.X, name)
	default:
		return false
	}
}

func fieldListContainsConnectRequest(fields *ast.FieldList) bool {
	for _, field := range fields.List {
		if strings.Contains(exprString(field.Type), "connect.Request[") {
			return true
		}
	}
	return false
}

func exprString(expr ast.Expr) string {
	var b strings.Builder
	if err := printer.Fprint(&b, token.NewFileSet(), expr); err != nil {
		return ""
	}
	return b.String()
}

func projectTransportQueryPassthroughHandlers(t *testing.T, root string) map[string]bool {
	t.Helper()
	content, err := os.ReadFile(filepath.Join(root, "internal", "transport", "connect", "project_service.go"))
	if err != nil {
		t.Fatalf("read internal/transport/connect/project_service.go: %v", err)
	}
	text := string(content)
	passthrough := map[string]string{
		"GetProject":    "return h.service.GetProject(ctx, req)",
		"ListProjects":  "return h.service.ListProjects(ctx, req)",
		"RemoveProject": "return h.service.RemoveProject(ctx, req)",
	}
	result := make(map[string]bool, len(passthrough))
	for method, marker := range passthrough {
		result[method] = strings.Contains(text, marker)
	}
	return result
}

func goFileImports(t *testing.T, file string) []string {
	t.Helper()
	parsed, err := parser.ParseFile(token.NewFileSet(), file, nil, parser.ImportsOnly)
	if err != nil {
		t.Fatalf("parse imports from %s: %v", file, err)
	}
	imports := make([]string, 0, len(parsed.Imports))
	for _, spec := range parsed.Imports {
		imported, err := strconv.Unquote(spec.Path.Value)
		if err != nil {
			t.Fatalf("parse import path %s in %s: %v", spec.Path.Value, file, err)
		}
		imports = append(imports, imported)
	}
	return imports
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
