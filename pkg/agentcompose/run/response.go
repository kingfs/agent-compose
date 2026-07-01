package run

import (
	"time"

	"agent-compose/pkg/agentcompose/project"
	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

func DetailResponse(record project.RunRecord) *agentcomposev2.RunDetail {
	return &agentcomposev2.RunDetail{
		Summary:      SummaryResponse(record),
		Prompt:       record.Prompt,
		Output:       record.Output,
		ResultJson:   record.ResultJSON,
		LogsPath:     record.LogsPath,
		ArtifactsDir: record.ArtifactsDir,
		CleanupError: record.CleanupError,
		Driver:       record.Driver,
		ImageRef:     record.ImageRef,
	}
}

func SummaryResponse(record project.RunRecord) *agentcomposev2.RunSummary {
	return &agentcomposev2.RunSummary{
		RunId:           record.RunID,
		ProjectId:       record.ProjectID,
		ProjectName:     record.ProjectName,
		ProjectRevision: uint64(record.ProjectRevision),
		AgentId:         record.ManagedAgentID,
		AgentName:       record.AgentName,
		Source:          SourceResponse(record.Source),
		SchedulerId:     record.SchedulerID,
		TriggerId:       record.TriggerID,
		Status:          StatusResponse(record.Status),
		SessionId:       record.SessionID,
		ExitCode:        int32(record.ExitCode),
		Error:           record.Error,
		StartedAt:       FormatProjectTime(record.StartedAt),
		CompletedAt:     FormatProjectTime(record.CompletedAt),
		DurationMs:      record.DurationMs,
		CreatedAt:       FormatProjectTime(record.CreatedAt),
		UpdatedAt:       FormatProjectTime(record.UpdatedAt),
	}
}

func StatusResponse(status string) agentcomposev2.RunStatus {
	switch project.NormalizeRunStatus(status) {
	case project.RunStatusPending:
		return agentcomposev2.RunStatus_RUN_STATUS_PENDING
	case project.RunStatusRunning:
		return agentcomposev2.RunStatus_RUN_STATUS_RUNNING
	case project.RunStatusSucceeded:
		return agentcomposev2.RunStatus_RUN_STATUS_SUCCEEDED
	case project.RunStatusFailed:
		return agentcomposev2.RunStatus_RUN_STATUS_FAILED
	case project.RunStatusCanceled:
		return agentcomposev2.RunStatus_RUN_STATUS_CANCELED
	default:
		return agentcomposev2.RunStatus_RUN_STATUS_UNSPECIFIED
	}
}

func StatusFromProto(status agentcomposev2.RunStatus) string {
	switch status {
	case agentcomposev2.RunStatus_RUN_STATUS_PENDING:
		return project.RunStatusPending
	case agentcomposev2.RunStatus_RUN_STATUS_RUNNING:
		return project.RunStatusRunning
	case agentcomposev2.RunStatus_RUN_STATUS_SUCCEEDED:
		return project.RunStatusSucceeded
	case agentcomposev2.RunStatus_RUN_STATUS_FAILED:
		return project.RunStatusFailed
	case agentcomposev2.RunStatus_RUN_STATUS_CANCELED:
		return project.RunStatusCanceled
	default:
		return ""
	}
}

func SourceResponse(source string) agentcomposev2.RunSource {
	switch project.NormalizeRunSource(source) {
	case project.RunSourceScheduler:
		return agentcomposev2.RunSource_RUN_SOURCE_SCHEDULER
	case project.RunSourceAPI:
		return agentcomposev2.RunSource_RUN_SOURCE_API
	case project.RunSourceManual:
		return agentcomposev2.RunSource_RUN_SOURCE_MANUAL
	default:
		return agentcomposev2.RunSource_RUN_SOURCE_UNSPECIFIED
	}
}

func SourceFromProto(source agentcomposev2.RunSource) string {
	switch source {
	case agentcomposev2.RunSource_RUN_SOURCE_SCHEDULER:
		return project.RunSourceScheduler
	case agentcomposev2.RunSource_RUN_SOURCE_API:
		return project.RunSourceAPI
	case agentcomposev2.RunSource_RUN_SOURCE_MANUAL:
		return project.RunSourceManual
	default:
		return project.RunSourceManual
	}
}

func SourceFilterFromProto(source agentcomposev2.RunSource) string {
	switch source {
	case agentcomposev2.RunSource_RUN_SOURCE_SCHEDULER:
		return project.RunSourceScheduler
	case agentcomposev2.RunSource_RUN_SOURCE_API:
		return project.RunSourceAPI
	case agentcomposev2.RunSource_RUN_SOURCE_MANUAL:
		return project.RunSourceManual
	default:
		return ""
	}
}

func FormatProjectTime(value time.Time) string {
	if value.IsZero() {
		return ""
	}
	return value.UTC().Format(time.RFC3339Nano)
}

func CleanupPolicyStopsSession(policy agentcomposev2.RunSessionCleanupPolicy) bool {
	return policy != agentcomposev2.RunSessionCleanupPolicy_RUN_SESSION_CLEANUP_POLICY_KEEP_RUNNING
}
