package agentcompose

import (
	"time"

	projectdomain "agent-compose/internal/agentcompose/project"
	"agent-compose/pkg/compose"
	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

func projectResponse(project ProjectRecord, spec *agentcomposev2.ProjectSpec, agents []ProjectAgentRecord, schedulers []ProjectSchedulerRecord) *agentcomposev2.Project {
	return &agentcomposev2.Project{
		Summary:    projectSummaryResponse(project, agents, schedulers),
		Spec:       spec,
		Agents:     projectAgentResponses(agents),
		Schedulers: projectSchedulerResponses(schedulers),
	}
}

func projectSummaryResponse(project ProjectRecord, agents []ProjectAgentRecord, schedulers []ProjectSchedulerRecord) *agentcomposev2.ProjectSummary {
	return &agentcomposev2.ProjectSummary{
		ProjectId:       project.ID,
		Name:            project.Name,
		SourcePath:      project.SourcePath,
		CurrentRevision: uint64(project.CurrentRevision),
		SpecHash:        project.SpecHash,
		AgentCount:      uint32(len(agents)),
		SchedulerCount:  uint32(len(schedulers)),
		CreatedAt:       formatProjectTime(project.CreatedAt),
		UpdatedAt:       formatProjectTime(project.UpdatedAt),
		RemovedAt:       formatProjectTime(project.RemovedAt),
	}
}

func projectRevisionResponse(revision ProjectRevisionRecord, spec *agentcomposev2.ProjectSpec) *agentcomposev2.ProjectRevision {
	return &agentcomposev2.ProjectRevision{
		ProjectId: revision.ProjectID,
		Revision:  uint64(revision.Revision),
		SpecHash:  revision.SpecHash,
		Spec:      spec,
		CreatedAt: formatProjectTime(revision.CreatedAt),
	}
}

func projectAgentResponses(agents []ProjectAgentRecord) []*agentcomposev2.ProjectAgent {
	items := make([]*agentcomposev2.ProjectAgent, 0, len(agents))
	for _, agent := range agents {
		items = append(items, &agentcomposev2.ProjectAgent{
			ProjectId:        agent.ProjectID,
			AgentName:        agent.AgentName,
			ManagedAgentId:   agent.ManagedAgentID,
			Provider:         agent.Provider,
			Model:            agent.Model,
			Image:            agent.Image,
			Driver:           agent.Driver,
			SchedulerEnabled: agent.SchedulerEnabled,
		})
	}
	return items
}

func projectSchedulerResponses(schedulers []ProjectSchedulerRecord) []*agentcomposev2.ProjectScheduler {
	items := make([]*agentcomposev2.ProjectScheduler, 0, len(schedulers))
	for _, scheduler := range schedulers {
		items = append(items, &agentcomposev2.ProjectScheduler{
			ProjectId:       scheduler.ProjectID,
			AgentName:       scheduler.AgentName,
			SchedulerId:     scheduler.SchedulerID,
			ManagedLoaderId: scheduler.ManagedLoaderID,
			Enabled:         scheduler.Enabled,
			TriggerCount:    uint32(scheduler.TriggerCount),
		})
	}
	return items
}

// ProjectSpecResponse converts a normalized compose spec into the v2 ProjectSpec API shape.
func ProjectSpecResponse(spec *compose.NormalizedProjectSpec) *agentcomposev2.ProjectSpec {
	return projectdomain.ProjectSpecResponse(spec)
}

func formatProjectTime(value time.Time) string {
	return projectdomain.FormatTime(value)
}
