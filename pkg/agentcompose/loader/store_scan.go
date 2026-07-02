package loader

import (
	"fmt"
	"strings"
)

func scanLoaderSummary(scan func(dest ...any) error) (LoaderSummary, error) {
	var item LoaderSummary
	var enabled int
	var capsetIDsRaw string
	var createdAtRaw any
	var updatedAtRaw any
	var latestRunAtRaw any
	if err := scan(
		&item.ID,
		&item.Name,
		&item.Description,
		&item.Runtime,
		&item.WorkspaceID,
		&item.AgentID,
		&item.Driver,
		&item.GuestImage,
		&item.DefaultAgent,
		&item.SessionPolicy,
		&item.ConcurrencyPolicy,
		&capsetIDsRaw,
		&item.ManagedProjectID,
		&item.ManagedRevision,
		&item.ManagedAgentName,
		&item.ManagedSchedulerID,
		&enabled,
		&item.LastError,
		&createdAtRaw,
		&updatedAtRaw,
		&item.TriggerCount,
		&item.RunCount,
		&item.EventCount,
		&latestRunAtRaw,
	); err != nil {
		return LoaderSummary{}, fmt.Errorf("scan loader summary: %w", err)
	}
	item.CapsetIDs = decodeCapsetIDs(capsetIDsRaw)
	item.ManagedProjectID = strings.TrimSpace(item.ManagedProjectID)
	item.ManagedAgentName = strings.TrimSpace(item.ManagedAgentName)
	item.ManagedSchedulerID = strings.TrimSpace(item.ManagedSchedulerID)
	item.Enabled = enabled != 0
	item.CreatedAt = parseStoredTime(createdAtRaw)
	item.UpdatedAt = parseStoredTime(updatedAtRaw)
	item.LatestRunAt = parseStoredTime(latestRunAtRaw)
	return item, nil
}

func scanLoader(scan func(dest ...any) error) (Loader, error) {
	var item Loader
	var enabled int
	var envJSON string
	var capsetIDsRaw string
	var createdAtRaw any
	var updatedAtRaw any
	if err := scan(
		&item.Summary.ID,
		&item.Summary.Name,
		&item.Summary.Description,
		&item.Summary.Runtime,
		&item.Script,
		&item.Summary.WorkspaceID,
		&item.Summary.AgentID,
		&item.Summary.Driver,
		&item.Summary.GuestImage,
		&item.Summary.DefaultAgent,
		&item.Summary.SessionPolicy,
		&item.Summary.ConcurrencyPolicy,
		&capsetIDsRaw,
		&envJSON,
		&item.Summary.ManagedProjectID,
		&item.Summary.ManagedRevision,
		&item.Summary.ManagedAgentName,
		&item.Summary.ManagedSchedulerID,
		&enabled,
		&item.Summary.LastError,
		&createdAtRaw,
		&updatedAtRaw,
	); err != nil {
		return Loader{}, fmt.Errorf("scan loader: %w", err)
	}
	item.Summary.CapsetIDs = decodeCapsetIDs(capsetIDsRaw)
	item.Summary.ManagedProjectID = strings.TrimSpace(item.Summary.ManagedProjectID)
	item.Summary.ManagedAgentName = strings.TrimSpace(item.Summary.ManagedAgentName)
	item.Summary.ManagedSchedulerID = strings.TrimSpace(item.Summary.ManagedSchedulerID)
	item.Summary.Enabled = enabled != 0
	item.Summary.CreatedAt = parseStoredTime(createdAtRaw)
	item.Summary.UpdatedAt = parseStoredTime(updatedAtRaw)
	envItems, err := decodeLoaderEnvItems(envJSON)
	if err != nil {
		return Loader{}, err
	}
	item.EnvItems = envItems
	return item, nil
}

func scanLoaderTrigger(scan func(dest ...any) error) (LoaderTrigger, error) {
	var item LoaderTrigger
	var enabled int
	var autoID int
	var nextFireAtRaw any
	var lastFiredAtRaw any
	if err := scan(&item.LoaderID, &item.ID, &item.Kind, &item.Topic, &item.IntervalMs, &enabled, &autoID, &item.SpecJSON, &nextFireAtRaw, &lastFiredAtRaw); err != nil {
		return LoaderTrigger{}, fmt.Errorf("scan loader trigger: %w", err)
	}
	item.Enabled = enabled != 0
	item.AutoID = autoID != 0
	item.NextFireAt = parseStoredLoaderTriggerTime(nextFireAtRaw)
	item.LastFiredAt = parseStoredLoaderTriggerTime(lastFiredAtRaw)
	return item, nil
}

func scanLoaderRun(scan func(dest ...any) error) (LoaderRunSummary, error) {
	var item LoaderRunSummary
	var startedAtRaw any
	var completedAtRaw any
	if err := scan(&item.LoaderID, &item.ID, &item.TriggerID, &item.TriggerKind, &item.TriggerSource, &item.Status, &startedAtRaw, &completedAtRaw, &item.DurationMs, &item.Error, &item.ResultJSON, &item.PayloadJSON, &item.SourceScriptHash, &item.ArtifactsDir); err != nil {
		return LoaderRunSummary{}, fmt.Errorf("scan loader run: %w", err)
	}
	item.StartedAt = parseStoredTime(startedAtRaw)
	item.CompletedAt = parseStoredTime(completedAtRaw)
	return item, nil
}

func scanLoaderEvent(scan func(dest ...any) error) (LoaderEvent, error) {
	var item LoaderEvent
	var createdAtRaw any
	if err := scan(&item.LoaderID, &item.ID, &item.RunID, &item.TriggerID, &item.Type, &item.Level, &item.Message, &item.PayloadJSON, &item.LinkedSessionID, &item.LinkedCellID, &item.LinkedAgentSessionID, &createdAtRaw); err != nil {
		return LoaderEvent{}, fmt.Errorf("scan loader event: %w", err)
	}
	item.CreatedAt = parseStoredTime(createdAtRaw)
	return item, nil
}

func scanLoaderBinding(scan func(dest ...any) error) (LoaderBinding, error) {
	var item LoaderBinding
	var createdAtRaw any
	var updatedAtRaw any
	if err := scan(&item.LoaderID, &item.SessionID, &createdAtRaw, &updatedAtRaw); err != nil {
		return LoaderBinding{}, fmt.Errorf("scan loader binding: %w", err)
	}
	item.CreatedAt = parseStoredTime(createdAtRaw)
	item.UpdatedAt = parseStoredTime(updatedAtRaw)
	return item, nil
}
