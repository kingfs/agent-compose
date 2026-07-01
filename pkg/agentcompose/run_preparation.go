package agentcompose

import (
	"context"
	"fmt"
	"os"

	projectpkg "agent-compose/pkg/agentcompose/project"
	runpkg "agent-compose/pkg/agentcompose/run"
	"agent-compose/pkg/compose"
	appconfig "agent-compose/pkg/config"
	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

type ProjectRunPreparation struct {
	EnvItems         []SessionEnvVar
	ProviderEnvItems []SessionEnvVar
	CapsetIDs        []string
	WorkspaceConfig  *WorkspaceConfig
	Workspace        *SessionWorkspace
}

func (s *Service) prepareProjectRun(ctx context.Context, run ProjectRunRecord, requestEnv []*agentcomposev2.EnvVarSpec) (ProjectRunPreparation, error) {
	if s == nil || s.configDB == nil {
		return ProjectRunPreparation{}, fmt.Errorf("config store is required")
	}
	prepared, err := runpkg.PrepareProjectRun(ctx, runPreparationStore{store: s.configDB}, runWorkspacePreparer{service: s}, projectpkg.RunRecord(run), requestEnv)
	if err != nil {
		return ProjectRunPreparation{}, err
	}
	result := ProjectRunPreparation{
		EnvItems:         filterPersistedRuntimeEnv(sessionEnvItemsFromRun(prepared.EnvItems)),
		ProviderEnvItems: sessionEnvItemsFromRun(prepared.ProviderEnvItems),
		CapsetIDs:        prepared.CapsetIDs,
	}
	if prepared.WorkspaceConfig != nil {
		config := workspaceConfigFromRun(*prepared.WorkspaceConfig)
		result.WorkspaceConfig = &config
		result.Workspace = toSessionWorkspaceSnapshot(config)
	}
	return result, nil
}

type runPreparationStore struct {
	store *ConfigStore
}

func (s runPreparationStore) GetProject(ctx context.Context, projectID string) (projectpkg.ProjectRecord, error) {
	record, err := s.store.GetProject(ctx, projectID)
	return projectpkg.ProjectRecord(record), err
}

func (s runPreparationStore) GetProjectRevision(ctx context.Context, projectID string, revision int64) (runpkg.RevisionRecord, error) {
	record, err := s.store.GetProjectRevision(ctx, projectID, revision)
	return runpkg.RevisionRecord{SpecJSON: record.SpecJSON}, err
}

func (s runPreparationStore) GetManagedAgentDefinition(ctx context.Context, agentID string) (runpkg.AgentDefinition, error) {
	agent, err := s.store.GetAgentDefinition(ctx, agentID)
	return runpkg.AgentDefinition{EnvItems: runEnvItemsFromSession(agent.EnvItems), CapsetIDs: agent.CapsetIDs}, err
}

func (s runPreparationStore) ListGlobalEnv(ctx context.Context) ([]runpkg.EnvItem, error) {
	items, err := s.store.ListGlobalEnv(ctx)
	return runEnvItemsFromSession(items), err
}

type runWorkspacePreparer struct {
	service *Service
}

func (p runWorkspacePreparer) PrepareProjectRunWorkspace(ctx context.Context, run projectpkg.RunRecord, project projectpkg.ProjectRecord, projectWorkspace, agentWorkspace *compose.WorkspaceSpec) (*runpkg.WorkspaceConfig, error) {
	return runpkg.PrepareWorkspace(ctx, p, run, project, projectWorkspace, agentWorkspace)
}

func (p runWorkspacePreparer) SessionWorkspaceSnapshot(config runpkg.WorkspaceConfig) *runpkg.WorkspaceSnapshot {
	return &runpkg.WorkspaceSnapshot{ID: config.ID}
}

func (p runWorkspacePreparer) MaterializeLocalProjectRunWorkspace(run projectpkg.RunRecord, project projectpkg.ProjectRecord, workspace *compose.WorkspaceSpec) (runpkg.WorkspaceConfig, error) {
	config, err := p.service.materializeLocalProjectRunWorkspace(ProjectRunRecord(run), ProjectRecord(project), workspace)
	return runWorkspaceConfigFromRoot(config), err
}

func runEnvItemsFromSession(items []SessionEnvVar) []runpkg.EnvItem {
	converted := make([]runpkg.EnvItem, 0, len(items))
	for _, item := range items {
		converted = append(converted, runpkg.EnvItem{Name: item.Name, Value: item.Value, Secret: item.Secret})
	}
	return converted
}

func sessionEnvItemsFromRun(items []runpkg.EnvItem) []SessionEnvVar {
	converted := make([]SessionEnvVar, 0, len(items))
	for _, item := range items {
		converted = append(converted, SessionEnvVar{Name: item.Name, Value: item.Value, Secret: item.Secret})
	}
	return normalizeEnvItems(converted)
}

func runWorkspaceConfigFromRoot(config WorkspaceConfig) runpkg.WorkspaceConfig {
	return runpkg.WorkspaceConfig{ID: config.ID, Name: config.Name, Type: config.Type, ConfigJSON: config.ConfigJSON, Comment: config.Comment}
}

func workspaceConfigFromRun(config runpkg.WorkspaceConfig) WorkspaceConfig {
	return WorkspaceConfig{ID: config.ID, Name: config.Name, Type: config.Type, ConfigJSON: config.ConfigJSON, Comment: config.Comment}
}

func decodeProjectRevisionSpec(raw string) (*agentcomposev2.ProjectSpec, error) {
	return runpkg.DecodeProjectRevisionSpec(raw)
}

func normalizedProjectAgentByName(spec *agentcomposev2.ProjectSpec, name string) (*agentcomposev2.AgentSpec, bool) {
	return runpkg.NormalizedProjectAgentByName(spec, name)
}

func sessionEnvItemsFromV2(items []*agentcomposev2.EnvVarSpec) []SessionEnvVar {
	return sessionEnvItemsFromRun(runpkg.EnvItemsFromV2(items))
}

func composeWorkspaceSpecFromV2(workspace *agentcomposev2.WorkspaceSpec) *compose.WorkspaceSpec {
	return runpkg.WorkspaceSpecFromV2(workspace)
}

func mergeRunEnvItems(groups ...[]SessionEnvVar) []SessionEnvVar {
	converted := make([][]runpkg.EnvItem, 0, len(groups))
	for _, group := range groups {
		converted = append(converted, runEnvItemsFromSession(group))
	}
	return sessionEnvItemsFromRun(runpkg.MergeEnvItems(converted...))
}

func (s *Service) prepareProjectRunWorkspace(ctx context.Context, run ProjectRunRecord, project ProjectRecord, projectWorkspace, agentWorkspace *compose.WorkspaceSpec) (*WorkspaceConfig, error) {
	config, err := runpkg.PrepareWorkspace(ctx, runWorkspacePreparer{service: s}, projectpkg.RunRecord(run), projectpkg.ProjectRecord(project), projectWorkspace, agentWorkspace)
	if err != nil {
		return nil, err
	}
	if config == nil {
		return nil, nil
	}
	rootConfig := workspaceConfigFromRun(*config)
	return &rootConfig, nil
}

func (s *Service) materializeLocalProjectRunWorkspace(run ProjectRunRecord, project ProjectRecord, workspace *compose.WorkspaceSpec) (WorkspaceConfig, error) {
	if s == nil || s.config == nil {
		return WorkspaceConfig{}, fmt.Errorf("config is required")
	}
	sourceDir, err := resolveLocalProjectWorkspacePath(project, workspace.Path)
	if err != nil {
		return WorkspaceConfig{}, err
	}
	workspaceID := projectRunWorkspaceID(run, "local")
	configJSON := defaultFileWorkspaceConfigJSON(s.config, workspaceID)
	if _, err := validateFileWorkspaceConfig(s.config, workspaceID, configJSON); err != nil {
		return WorkspaceConfig{}, err
	}
	if err := resetFileWorkspaceSnapshotContent(s.config, workspaceID); err != nil {
		return WorkspaceConfig{}, err
	}
	config := WorkspaceConfig{
		ID:         workspaceID,
		Name:       projectRunWorkspaceName(run, "local"),
		Type:       "file",
		ConfigJSON: configJSON,
		Comment:    fmt.Sprintf("project run %s local workspace snapshot", run.RunID),
	}
	content, err := openFileWorkspaceContent(s.config, config)
	if err != nil {
		return WorkspaceConfig{}, err
	}
	defer func() { _ = content.Root.Close() }()
	sourceRoot, err := os.OpenRoot(sourceDir)
	if err != nil {
		return WorkspaceConfig{}, fmt.Errorf("open local workspace source %s: %w", sourceDir, err)
	}
	defer func() { _ = sourceRoot.Close() }()
	if err := copyRootDirectoryContents(sourceRoot, content.AbsRoot); err != nil {
		return WorkspaceConfig{}, fmt.Errorf("materialize local workspace snapshot: %w", err)
	}
	return config, nil
}

func resolveLocalProjectWorkspacePath(project ProjectRecord, rawPath string) (string, error) {
	return runpkg.ResolveLocalWorkspacePath(projectpkg.ProjectRecord(project), rawPath)
}

func cleanLocalWorkspacePath(raw string) (string, error) {
	return runpkg.CleanLocalWorkspacePath(raw)
}

func projectRunGitWorkspaceConfig(run ProjectRunRecord, workspace *compose.WorkspaceSpec) (WorkspaceConfig, error) {
	config, err := runpkg.GitWorkspaceConfig(projectpkg.RunRecord(run), workspace)
	return workspaceConfigFromRun(config), err
}

func projectRunWorkspaceID(run ProjectRunRecord, provider string) string {
	return runpkg.WorkspaceID(projectpkg.RunRecord(run), provider)
}

func projectRunWorkspaceName(run ProjectRunRecord, provider string) string {
	return runpkg.WorkspaceName(projectpkg.RunRecord(run), provider)
}

func resetFileWorkspaceSnapshotContent(config *appconfig.Config, workspaceID string) error {
	dataRoot, err := openFileWorkspaceDataRoot(config)
	if err != nil {
		return err
	}
	defer func() { _ = dataRoot.Close() }()
	relRoot, err := fileWorkspaceContentRelRoot(workspaceID)
	if err != nil {
		return err
	}
	if err := dataRoot.RemoveAll(relRoot); err != nil {
		return fmt.Errorf("reset local workspace snapshot %s: %w", workspaceID, err)
	}
	return nil
}
