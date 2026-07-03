package connecttransport

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"

	projectdomain "agent-compose/internal/project"
	"agent-compose/internal/projecttypes"
	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

func GetProjectRequestFromProto(req *agentcomposev2.GetProjectRequest) projectdomain.GetProjectRequest {
	if req == nil {
		return projectdomain.GetProjectRequest{}
	}
	return projectdomain.GetProjectRequest{
		Ref:         ProjectRefFromProto(req.GetProject()),
		IncludeSpec: req.GetIncludeSpec(),
	}
}

func ListProjectsRequestFromProto(req *agentcomposev2.ListProjectsRequest) projectdomain.ListProjectsRequest {
	if req == nil {
		return projectdomain.ListProjectsRequest{}
	}
	return projectdomain.ListProjectsRequest{
		Query:          req.GetQuery(),
		IncludeRemoved: req.GetIncludeRemoved(),
		Offset:         int(req.GetOffset()),
		Limit:          int(req.GetLimit()),
	}
}

func RemoveProjectRequestFromProto(req *agentcomposev2.RemoveProjectRequest) projectdomain.RemoveProjectRequest {
	if req == nil {
		return projectdomain.RemoveProjectRequest{}
	}
	return projectdomain.RemoveProjectRequest{
		Ref:           ProjectRefFromProto(req.GetProject()),
		RemoveHistory: req.GetRemoveHistory(),
	}
}

func ProjectRefFromProto(ref *agentcomposev2.ProjectRef) projectdomain.ProjectRef {
	if ref == nil {
		return projectdomain.ProjectRef{}
	}
	return projectdomain.ProjectRef{
		ProjectID:  ref.GetProjectId(),
		Name:       ref.GetName(),
		SourcePath: ref.GetSourcePath(),
	}
}

func GetProjectResponseFromResult(result projectdomain.GetProjectResult) (*agentcomposev2.GetProjectResponse, error) {
	spec, err := projectSpecFromRevisionJSON(result.RevisionSpecJSON)
	if err != nil {
		return nil, fmt.Errorf("decode project %s revision %d: %w", result.Project.Name, result.Project.CurrentRevision, err)
	}
	return &agentcomposev2.GetProjectResponse{
		Project: projectResponse(result.Project, spec, result.Agents, result.Schedulers),
	}, nil
}

func ListProjectsResponseFromResult(result projectdomain.ListProjectsResult) *agentcomposev2.ListProjectsResponse {
	resp := &agentcomposev2.ListProjectsResponse{
		TotalCount: uint32(result.TotalCount),
		HasMore:    result.HasMore,
		NextOffset: uint32(result.NextOffset),
	}
	for _, project := range result.Projects {
		resp.Projects = append(resp.Projects, projectSummaryResponse(project, nil, nil))
	}
	return resp
}

func RemoveProjectResponseFromResult(result projectdomain.RemoveProjectResult) *agentcomposev2.RemoveProjectResponse {
	return &agentcomposev2.RemoveProjectResponse{
		Project: projectResponse(result.Project, nil, result.Agents, result.Schedulers),
		Changes: projectChangesToProto(result.Changes),
	}
}

func ProjectQueryConnectCode(err error) connect.Code {
	switch projectdomain.ErrorKindOf(err) {
	case projectdomain.ErrorKindValidation:
		return connect.CodeInvalidArgument
	case projectdomain.ErrorKindNotFound:
		return connect.CodeNotFound
	case projectdomain.ErrorKindUnimplemented:
		return connect.CodeUnimplemented
	case projectdomain.ErrorKindRuntime, projectdomain.ErrorKindStorage, projectdomain.ErrorKindUnknown:
		return connect.CodeInternal
	default:
		return connect.CodeInternal
	}
}

func projectSpecFromRevisionJSON(raw string) (*agentcomposev2.ProjectSpec, error) {
	if strings.TrimSpace(raw) == "" {
		return nil, nil
	}
	var spec agentcomposev2.ProjectSpec
	if err := json.Unmarshal([]byte(strings.TrimSpace(raw)), &spec); err != nil {
		return nil, fmt.Errorf("decode project revision spec: %w", err)
	}
	return &spec, nil
}

func projectResponse(project projecttypes.ProjectRecord, spec *agentcomposev2.ProjectSpec, agents []projecttypes.ProjectAgentRecord, schedulers []projecttypes.ProjectSchedulerRecord) *agentcomposev2.Project {
	return &agentcomposev2.Project{
		Summary:    projectSummaryResponse(project, agents, schedulers),
		Spec:       spec,
		Agents:     projectAgentResponses(agents),
		Schedulers: projectSchedulerResponses(schedulers),
	}
}

func projectSummaryResponse(project projecttypes.ProjectRecord, agents []projecttypes.ProjectAgentRecord, schedulers []projecttypes.ProjectSchedulerRecord) *agentcomposev2.ProjectSummary {
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

func projectAgentResponses(agents []projecttypes.ProjectAgentRecord) []*agentcomposev2.ProjectAgent {
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

func projectSchedulerResponses(schedulers []projecttypes.ProjectSchedulerRecord) []*agentcomposev2.ProjectScheduler {
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

func projectChangesToProto(changes []projectdomain.Change) []*agentcomposev2.ProjectChange {
	if len(changes) == 0 {
		return nil
	}
	result := make([]*agentcomposev2.ProjectChange, 0, len(changes))
	for _, change := range changes {
		result = append(result, &agentcomposev2.ProjectChange{
			Action:       projectChangeActionToProto(change.Kind),
			ResourceType: change.Resource,
			ResourceId:   change.ID,
			Name:         change.Name,
			Message:      change.Message,
		})
	}
	return result
}

func projectChangeActionToProto(kind projectdomain.ChangeKind) agentcomposev2.ProjectChangeAction {
	switch kind {
	case projectdomain.ChangeKindCreated:
		return agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_CREATED
	case projectdomain.ChangeKindUpdated:
		return agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_UPDATED
	case projectdomain.ChangeKindDeleted:
		return agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_REMOVED
	case projectdomain.ChangeKindUnchanged:
		return agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_UNCHANGED
	default:
		return agentcomposev2.ProjectChangeAction_PROJECT_CHANGE_ACTION_UNSPECIFIED
	}
}

func formatProjectTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}
