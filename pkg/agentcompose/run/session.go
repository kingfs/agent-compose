package run

import (
	"fmt"
	"strings"

	"agent-compose/pkg/agentcompose/project"
)

type SessionTag struct {
	Name  string
	Value string
}

func SessionTitle(run project.RunRecord) string {
	projectName := strings.TrimSpace(run.ProjectName)
	if projectName == "" {
		projectName = strings.TrimSpace(run.ProjectID)
	}
	agent := strings.TrimSpace(run.AgentName)
	if agent == "" {
		agent = "agent"
	}
	return strings.TrimSpace(fmt.Sprintf("%s/%s run", projectName, agent))
}

func SessionTags(run project.RunRecord) []SessionTag {
	tags := []SessionTag{
		{Name: "project", Value: strings.TrimSpace(run.ProjectID)},
		{Name: "agent", Value: strings.TrimSpace(run.AgentName)},
		{Name: "run_id", Value: strings.TrimSpace(run.RunID)},
		{Name: "source", Value: project.NormalizeRunSource(run.Source)},
	}
	if schedulerID := strings.TrimSpace(run.SchedulerID); schedulerID != "" {
		tags = append(tags, SessionTag{Name: "scheduler_id", Value: schedulerID})
	}
	return tags
}

func MergeSessionTags(existing, additions []SessionTag) []SessionTag {
	result := append([]SessionTag(nil), existing...)
	for _, addition := range additions {
		addition.Name = strings.TrimSpace(addition.Name)
		addition.Value = strings.TrimSpace(addition.Value)
		if addition.Name == "" {
			continue
		}
		found := false
		for _, current := range result {
			if strings.TrimSpace(current.Name) == addition.Name && strings.TrimSpace(current.Value) == addition.Value {
				found = true
				break
			}
		}
		if !found {
			result = append(result, addition)
		}
	}
	return result
}
