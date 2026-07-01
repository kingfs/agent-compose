package run

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"agent-compose/pkg/agentcompose/project"
	"agent-compose/pkg/compose"
	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

type EnvItem struct {
	Name   string
	Value  string
	Secret bool
}

type WorkspaceConfig struct {
	ID         string
	Name       string
	Type       string
	ConfigJSON string
	Comment    string
}

type WorkspaceSnapshot struct {
	ID string
}

type Preparation struct {
	EnvItems         []EnvItem
	ProviderEnvItems []EnvItem
	CapsetIDs        []string
	WorkspaceConfig  *WorkspaceConfig
	Workspace        *WorkspaceSnapshot
}

type RevisionRecord struct {
	SpecJSON string
}

type AgentDefinition struct {
	EnvItems  []EnvItem
	CapsetIDs []string
}

type PreparationStore interface {
	GetProject(ctx context.Context, projectID string) (project.ProjectRecord, error)
	GetProjectRevision(ctx context.Context, projectID string, revision int64) (RevisionRecord, error)
	GetManagedAgentDefinition(ctx context.Context, agentID string) (AgentDefinition, error)
	ListGlobalEnv(ctx context.Context) ([]EnvItem, error)
}

type WorkspacePreparer interface {
	PrepareProjectRunWorkspace(ctx context.Context, run project.RunRecord, project project.ProjectRecord, projectWorkspace, agentWorkspace *compose.WorkspaceSpec) (*WorkspaceConfig, error)
	SessionWorkspaceSnapshot(config WorkspaceConfig) *WorkspaceSnapshot
}

func PrepareProjectRun(ctx context.Context, store PreparationStore, workspaces WorkspacePreparer, run project.RunRecord, requestEnv []*agentcomposev2.EnvVarSpec) (Preparation, error) {
	if store == nil {
		return Preparation{}, fmt.Errorf("config store is required")
	}
	projectRecord, err := store.GetProject(ctx, run.ProjectID)
	if err != nil {
		return Preparation{}, fmt.Errorf("resolve project %s: %w", run.ProjectID, err)
	}
	revision, err := store.GetProjectRevision(ctx, run.ProjectID, run.ProjectRevision)
	if err != nil {
		return Preparation{}, fmt.Errorf("resolve project revision %s/%d: %w", run.ProjectID, run.ProjectRevision, err)
	}
	spec, err := DecodeProjectRevisionSpec(revision.SpecJSON)
	if err != nil {
		return Preparation{}, err
	}
	agentSpec, ok := NormalizedProjectAgentByName(spec, run.AgentName)
	if !ok {
		return Preparation{}, fmt.Errorf("project revision %s/%d missing agent %s", run.ProjectID, run.ProjectRevision, run.AgentName)
	}
	agent, err := store.GetManagedAgentDefinition(ctx, run.ManagedAgentID)
	if err != nil {
		return Preparation{}, fmt.Errorf("resolve managed agent definition %s: %w", run.ManagedAgentID, err)
	}
	globalEnv, err := store.ListGlobalEnv(ctx)
	if err != nil {
		return Preparation{}, fmt.Errorf("list global env: %w", err)
	}
	envItems := MergeEnvItems(globalEnv, EnvItemsFromV2(spec.GetVariables()), agent.EnvItems, EnvItemsFromV2(requestEnv))
	providerEnvItems := envItems
	if workspaces == nil {
		return Preparation{}, fmt.Errorf("workspace preparer is required")
	}
	workspace, err := workspaces.PrepareProjectRunWorkspace(ctx, run, projectRecord, WorkspaceSpecFromV2(spec.GetWorkspace()), WorkspaceSpecFromV2(agentSpec.GetWorkspace()))
	if err != nil {
		return Preparation{}, err
	}
	prepared := Preparation{EnvItems: envItems, ProviderEnvItems: providerEnvItems, CapsetIDs: NormalizeStrings(agent.CapsetIDs)}
	if workspace != nil {
		prepared.WorkspaceConfig = workspace
		prepared.Workspace = workspaces.SessionWorkspaceSnapshot(*workspace)
	}
	return prepared, nil
}

func DecodeProjectRevisionSpec(raw string) (*agentcomposev2.ProjectSpec, error) {
	var spec agentcomposev2.ProjectSpec
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &spec); err != nil {
		return nil, fmt.Errorf("decode project revision spec: %w", err)
	}
	return &spec, nil
}

func NormalizedProjectAgentByName(spec *agentcomposev2.ProjectSpec, name string) (*agentcomposev2.AgentSpec, bool) {
	if spec == nil {
		return nil, false
	}
	name = strings.TrimSpace(name)
	for _, agent := range spec.GetAgents() {
		if agent.GetName() == name {
			return agent, true
		}
	}
	return nil, false
}

func EnvItemsFromV2(items []*agentcomposev2.EnvVarSpec) []EnvItem {
	env := make([]EnvItem, 0, len(items))
	for _, item := range items {
		if item == nil {
			continue
		}
		env = append(env, EnvItem{Name: item.GetName(), Value: item.GetValue(), Secret: item.GetSecret()})
	}
	return NormalizeEnvItems(env)
}

func WorkspaceSpecFromV2(workspace *agentcomposev2.WorkspaceSpec) *compose.WorkspaceSpec {
	if workspace == nil {
		return nil
	}
	return &compose.WorkspaceSpec{Provider: workspace.GetProvider(), URL: workspace.GetUrl(), Branch: workspace.GetBranch(), Path: workspace.GetPath()}
}

func MergeEnvItems(groups ...[]EnvItem) []EnvItem {
	var merged []EnvItem
	for _, group := range groups {
		merged = mergeEnvItems(merged, group)
	}
	return merged
}

func NormalizeEnvItems(items []EnvItem) []EnvItem {
	return mergeEnvItems(nil, items)
}

func mergeEnvItems(existing, additions []EnvItem) []EnvItem {
	result := append([]EnvItem(nil), existing...)
	index := make(map[string]int, len(result))
	for i, item := range result {
		item.Name = strings.TrimSpace(item.Name)
		result[i] = item
		if item.Name != "" {
			index[item.Name] = i
		}
	}
	for _, item := range additions {
		item.Name = strings.TrimSpace(item.Name)
		if item.Name == "" {
			continue
		}
		if pos, ok := index[item.Name]; ok {
			result[pos] = item
			continue
		}
		index[item.Name] = len(result)
		result = append(result, item)
	}
	return result
}

func NormalizeStrings(values []string) []string {
	seen := make(map[string]struct{}, len(values))
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = strings.TrimSpace(value)
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		result = append(result, value)
	}
	return result
}

func PrepareWorkspace(ctx context.Context, materializer LocalWorkspaceMaterializer, run project.RunRecord, projectRecord project.ProjectRecord, projectWorkspace, agentWorkspace *compose.WorkspaceSpec) (*WorkspaceConfig, error) {
	_ = ctx
	workspace := projectWorkspace
	if agentWorkspace != nil {
		workspace = agentWorkspace
	}
	if workspace == nil {
		return nil, nil
	}
	provider := strings.ToLower(strings.TrimSpace(workspace.Provider))
	switch provider {
	case "local":
		if materializer == nil {
			return nil, fmt.Errorf("local workspace materializer is required")
		}
		config, err := materializer.MaterializeLocalProjectRunWorkspace(run, projectRecord, workspace)
		if err != nil {
			return nil, err
		}
		return &config, nil
	case "git":
		config, err := GitWorkspaceConfig(run, workspace)
		if err != nil {
			return nil, err
		}
		return &config, nil
	default:
		if provider == "" {
			return nil, fmt.Errorf("workspace provider is required")
		}
		return nil, fmt.Errorf("unsupported workspace provider %q", workspace.Provider)
	}
}

type LocalWorkspaceMaterializer interface {
	MaterializeLocalProjectRunWorkspace(run project.RunRecord, project project.ProjectRecord, workspace *compose.WorkspaceSpec) (WorkspaceConfig, error)
}

func ResolveLocalWorkspacePath(projectRecord project.ProjectRecord, rawPath string) (string, error) {
	cleanPath, err := CleanLocalWorkspacePath(rawPath)
	if err != nil {
		return "", err
	}
	sourcePath := strings.TrimSpace(projectRecord.SourcePath)
	if sourcePath == "" {
		return "", fmt.Errorf("local workspace requires project source path")
	}
	sourceAbs, err := filepath.Abs(sourcePath)
	if err != nil {
		return "", fmt.Errorf("resolve project source path %q: %w", sourcePath, err)
	}
	sourceDir := sourceAbs
	if info, err := os.Stat(sourceAbs); err == nil && !info.IsDir() {
		sourceDir = filepath.Dir(sourceAbs)
	} else if err != nil {
		sourceDir = filepath.Dir(sourceAbs)
	}
	target := sourceDir
	if cleanPath != "." {
		target = filepath.Join(sourceDir, cleanPath)
	}
	targetAbs, err := filepath.Abs(target)
	if err != nil {
		return "", fmt.Errorf("resolve local workspace path %q: %w", rawPath, err)
	}
	info, err := os.Lstat(targetAbs)
	if err != nil {
		return "", fmt.Errorf("local workspace source %s: %w", targetAbs, err)
	}
	if info.Mode()&os.ModeSymlink != 0 {
		return "", fmt.Errorf("local workspace source %s is a symlink", targetAbs)
	}
	if !info.IsDir() {
		return "", fmt.Errorf("local workspace source %s is not a directory", targetAbs)
	}
	return targetAbs, nil
}

func CleanLocalWorkspacePath(raw string) (string, error) {
	trimmed := strings.TrimSpace(raw)
	if trimmed == "" {
		return "", fmt.Errorf("local workspace path is required")
	}
	if filepath.IsAbs(trimmed) {
		return "", fmt.Errorf("local workspace path %q must be relative", trimmed)
	}
	clean := filepath.Clean(trimmed)
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("local workspace path %q escapes project source root", trimmed)
	}
	return clean, nil
}

func GitWorkspaceConfig(run project.RunRecord, workspace *compose.WorkspaceSpec) (WorkspaceConfig, error) {
	workspaceID := WorkspaceID(run, "git")
	if strings.TrimSpace(workspace.URL) == "" {
		return WorkspaceConfig{}, fmt.Errorf("git workspace url is required")
	}
	if _, err := NormalizeGitCloneTarget(workspaceID, workspace.Path); err != nil {
		return WorkspaceConfig{}, err
	}
	payload, err := json.Marshal(struct {
		URL         string `json:"url"`
		Branch      string `json:"branch,omitempty"`
		CloneTarget string `json:"path,omitempty"`
	}{URL: strings.TrimSpace(workspace.URL), Branch: strings.TrimSpace(workspace.Branch), CloneTarget: strings.TrimSpace(workspace.Path)})
	if err != nil {
		return WorkspaceConfig{}, fmt.Errorf("encode git workspace config: %w", err)
	}
	return WorkspaceConfig{ID: workspaceID, Name: WorkspaceName(run, "git"), Type: "git", ConfigJSON: string(payload), Comment: fmt.Sprintf("project run %s git workspace snapshot", run.RunID)}, nil
}

func WorkspaceID(run project.RunRecord, provider string) string {
	return project.StableReadableIDForWorkspace(run.AgentName+"-"+provider, run.RunID+"|workspace|"+provider)
}

func WorkspaceName(run project.RunRecord, provider string) string {
	name := strings.TrimSpace(run.ProjectName)
	if name == "" {
		name = strings.TrimSpace(run.ProjectID)
	}
	agent := strings.TrimSpace(run.AgentName)
	if agent == "" {
		agent = "agent"
	}
	return strings.TrimSpace(fmt.Sprintf("%s %s %s run workspace", name, agent, provider))
}

func NormalizeGitCloneTarget(workspaceID, target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return workspaceID, nil
	}
	if filepath.IsAbs(target) {
		return "", fmt.Errorf("git workspace clone target %q must be relative", target)
	}
	clean := filepath.Clean(target)
	if clean == "." {
		return workspaceID, nil
	}
	if clean == ".." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("git workspace clone target %q escapes workspace root", target)
	}
	return clean, nil
}
