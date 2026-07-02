package agentcompose

import (
	"fmt"
	"slices"
	"strings"

	projectdomain "agent-compose/internal/agentcompose/project"
	"agent-compose/pkg/compose"
)

type projectManagedSchedulerBuild struct {
	scheduler          ProjectSchedulerRecord
	loader             Loader
	validationTriggers []LoaderTrigger
}

func projectAgentRecordsFromSpec(projectID string, revision int64, spec *compose.NormalizedProjectSpec) ([]ProjectAgentRecord, error) {
	agents := make([]ProjectAgentRecord, 0, len(spec.Agents))
	for _, agent := range spec.Agents {
		record, err := NewProjectAgentRecordFromSpec(projectID, revision, agent)
		if err != nil {
			return nil, err
		}
		agents = append(agents, record)
	}
	return agents, nil
}

func projectManagedAgentDefinitionsFromSpec(project ProjectRecord, revision int64, spec *compose.NormalizedProjectSpec) ([]AgentDefinition, error) {
	agents := make([]AgentDefinition, 0, len(spec.Agents))
	for _, agent := range spec.Agents {
		record, err := projectManagedAgentDefinitionFromSpec(project, revision, agent)
		if err != nil {
			return nil, err
		}
		agents = append(agents, record)
	}
	return agents, nil
}

func projectManagedAgentDefinitionFromSpec(project ProjectRecord, revision int64, agent compose.NormalizedAgentSpec) (AgentDefinition, error) {
	managedAgentID, err := StableManagedAgentID(project.ID, agent.Name)
	if err != nil {
		return AgentDefinition{}, err
	}
	driver := ""
	if agent.Driver != nil {
		driver = agent.Driver.Name
	}
	return AgentDefinition{
		ID:                     managedAgentID,
		Name:                   agent.Name,
		Enabled:                true,
		Provider:               agent.Provider,
		Model:                  agent.Model,
		SystemPrompt:           agent.SystemPrompt,
		Driver:                 driver,
		GuestImage:             agent.Image,
		EnvItems:               sessionEnvItemsFromCompose(agent.Env),
		ConfigJSON:             "{}",
		CapsetIDs:              normalizeCapsetIDs(agent.CapsetIDs),
		ManagedProjectID:       project.ID,
		ManagedProjectRevision: revision,
		ManagedAgentName:       agent.Name,
	}, nil
}

func sessionEnvItemsFromCompose(values map[string]compose.EnvVarSpec) []SessionEnvVar {
	names := make([]string, 0, len(values))
	for name := range values {
		names = append(names, name)
	}
	slices.Sort(names)
	items := make([]SessionEnvVar, 0, len(values))
	for _, name := range names {
		value := values[name]
		items = append(items, SessionEnvVar{Name: name, Value: value.Value, Secret: value.Secret})
	}
	return items
}

func projectManagedSchedulerRecords(builds []projectManagedSchedulerBuild) []ProjectSchedulerRecord {
	schedulers := make([]ProjectSchedulerRecord, 0, len(builds))
	for _, build := range builds {
		schedulers = append(schedulers, build.scheduler)
	}
	return schedulers
}

func projectManagedSchedulerLoaders(builds []projectManagedSchedulerBuild) []Loader {
	loaders := make([]Loader, 0, len(builds))
	for _, build := range builds {
		loaders = append(loaders, build.loader)
	}
	return loaders
}

func projectManagedSchedulerBuildsFromSpec(project ProjectRecord, revision int64, spec *compose.NormalizedProjectSpec) ([]projectManagedSchedulerBuild, error) {
	builds := make([]projectManagedSchedulerBuild, 0)
	for _, agent := range spec.Agents {
		record, ok, err := NewProjectSchedulerRecordFromSpec(project.ID, revision, agent)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		loader, err := projectManagedLoaderFromScheduler(project, record, agent)
		if err != nil {
			return nil, err
		}
		builds = append(builds, projectManagedSchedulerBuild{
			scheduler:          record,
			loader:             loader,
			validationTriggers: loader.Triggers,
		})
	}
	return builds, nil
}

func projectManagedLoaderFromScheduler(project ProjectRecord, scheduler ProjectSchedulerRecord, agent compose.NormalizedAgentSpec) (Loader, error) {
	managedAgentID, err := StableManagedAgentID(project.ID, agent.Name)
	if err != nil {
		return Loader{}, err
	}
	driver := ""
	if agent.Driver != nil {
		driver = agent.Driver.Name
	}
	var triggers []LoaderTrigger
	script := agent.Scheduler.Script
	if strings.TrimSpace(script) == "" {
		var err error
		triggers, script, err = projectManagedLoaderTriggersAndScript(project.ID, agent.Name, "", agent.Scheduler)
		if err != nil {
			return Loader{}, err
		}
	}
	return Loader{
		Summary: LoaderSummary{
			ID:                 scheduler.ManagedLoaderID,
			Name:               fmt.Sprintf("%s/%s scheduler", project.Name, agent.Name),
			Enabled:            scheduler.Enabled,
			Runtime:            LoaderRuntimeScheduler,
			AgentID:            managedAgentID,
			Driver:             driver,
			GuestImage:         agent.Image,
			DefaultAgent:       agent.Provider,
			SessionPolicy:      LoaderSessionPolicyNew,
			ConcurrencyPolicy:  LoaderConcurrencyPolicySkip,
			CapsetIDs:          normalizeCapsetIDs(agent.CapsetIDs),
			ManagedProjectID:   project.ID,
			ManagedRevision:    scheduler.Revision,
			ManagedAgentName:   agent.Name,
			ManagedSchedulerID: scheduler.SchedulerID,
		},
		Script:   script,
		Triggers: triggers,
		EnvItems: sessionEnvItemsFromCompose(agent.Env),
	}, nil
}

func projectManagedLoaderTriggersAndScript(projectID, agentName, schedulerName string, scheduler *compose.NormalizedSchedulerSpec) ([]LoaderTrigger, string, error) {
	triggerBuilds, script, err := projectdomain.ManagedLoaderTriggersAndScript(projectID, agentName, schedulerName, scheduler)
	if err != nil {
		return nil, "", err
	}
	triggers := make([]LoaderTrigger, 0, len(triggerBuilds))
	for _, trigger := range triggerBuilds {
		triggers = append(triggers, loaderTriggerFromProjectBuild(trigger))
	}
	return triggers, script, nil
}

func projectManagedLoaderTriggerAndRegistration(id, agentName string, trigger compose.NormalizedTriggerSpec) (LoaderTrigger, string, error) {
	triggerBuild, err := projectdomain.ManagedLoaderTriggerAndRegistration(id, agentName, trigger)
	if err != nil {
		return LoaderTrigger{}, "", err
	}
	return loaderTriggerFromProjectBuild(triggerBuild), triggerBuild.Registration, nil
}

func loaderTriggerFromProjectBuild(trigger projectdomain.SchedulerTriggerBuild) LoaderTrigger {
	return LoaderTrigger{
		ID:         trigger.ID,
		Kind:       trigger.Kind,
		Topic:      trigger.Topic,
		IntervalMs: trigger.IntervalMs,
		Enabled:    true,
		SpecJSON:   trigger.SpecJSON,
	}
}

func jsStringLiteral(value string) string {
	return projectdomain.JSStringLiteral(value)
}
