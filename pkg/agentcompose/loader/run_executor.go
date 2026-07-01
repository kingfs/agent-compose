package loader

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

var ErrRunBusyForRetry = errors.New("loader is already running")

type RunOptions struct {
	AlreadyEntered bool
	RetryWhenBusy  bool
}

type PreparedRun struct {
	Loader      Loader
	Trigger     *LoaderTrigger
	Run         LoaderRunSummary
	PayloadJSON string
}

type RunController interface {
	EnterRun(loader Loader) bool
	LeaveRun(loaderID string)
	RunArtifactsDir(loaderID, runID string) string
	WriteRunArtifact(dir, name, content string) error
	CreateLoaderRun(ctx context.Context, run LoaderRunSummary) error
	UpdateLoaderRun(ctx context.Context, run LoaderRunSummary) error
	UpdateLoaderLastError(ctx context.Context, loaderID, lastError string) error
	UpdateTriggerEventDelivery(ctx context.Context, run LoaderRunSummary)
	NotifyDashboard(reason string)
	AddLoaderEvent(ctx context.Context, loaderID, runID, triggerID, eventType, level, message string, payload any, linkedSessionID, linkedCellID, linkedAgentSessionID string) error
	Refresh(ctx context.Context) error
	ExecuteLoader(ctx context.Context, request LoaderExecutionRequest, host LoaderHost) (LoaderExecutionResult, error)
	NewRunHost(loader Loader, run *LoaderRunSummary, payloadJSON string) LoaderHost
	CleanupRunHost(ctx context.Context, host LoaderHost)
}

type RunExecutor struct {
	controller RunController
}

func NewRunExecutor(controller RunController) *RunExecutor {
	return &RunExecutor{controller: controller}
}

func (e *RunExecutor) Run(ctx context.Context, loader Loader, trigger *LoaderTrigger, payloadJSON, source string, options RunOptions, triggerEventAck ...func(context.Context) error) (LoaderRunSummary, error) {
	prepared, err := e.Prepare(ctx, loader, trigger, payloadJSON, source, options)
	if err != nil {
		return LoaderRunSummary{}, err
	}
	if len(triggerEventAck) > 0 && triggerEventAck[0] != nil {
		if err := triggerEventAck[0](ctx); err != nil {
			slog.Warn("failed to mark loader topic event published", "topic", source, "error", err)
		}
	}
	return e.Execute(ctx, prepared)
}

func (e *RunExecutor) Prepare(ctx context.Context, loader Loader, trigger *LoaderTrigger, payloadJSON, source string, options RunOptions) (PreparedRun, error) {
	c := e.controller
	payloadJSON, err := NormalizeJSONDocument(payloadJSON)
	if err != nil {
		if options.AlreadyEntered {
			c.LeaveRun(loader.Summary.ID)
		}
		return PreparedRun{}, err
	}
	now := time.Now().UTC()
	run := LoaderRunSummary{
		ID:               uuid.NewString(),
		LoaderID:         loader.Summary.ID,
		TriggerSource:    strings.TrimSpace(source),
		Status:           LoaderRunStatusRunning,
		StartedAt:        now,
		PayloadJSON:      payloadJSON,
		SourceScriptHash: SourceSHA(loader.Script),
		ArtifactsDir:     c.RunArtifactsDir(loader.Summary.ID, ""),
	}
	if trigger != nil {
		run.TriggerID = trigger.ID
		run.TriggerKind = trigger.Kind
	}
	run.ArtifactsDir = c.RunArtifactsDir(loader.Summary.ID, run.ID)

	entered := options.AlreadyEntered
	if !entered && !c.EnterRun(loader) {
		if options.RetryWhenBusy {
			return PreparedRun{}, ErrRunBusyForRetry
		}
		if err := os.MkdirAll(run.ArtifactsDir, 0o755); err != nil {
			return PreparedRun{}, fmt.Errorf("create loader run artifacts dir: %w", err)
		}
		_ = c.WriteRunArtifact(run.ArtifactsDir, "payload.json", payloadJSON)
		run.Status = LoaderRunStatusSkipped
		run.CompletedAt = now
		run.Error = "loader is already running"
		if err := c.CreateLoaderRun(ctx, run); err != nil {
			return PreparedRun{}, err
		}
		c.UpdateTriggerEventDelivery(ctx, run)
		c.NotifyDashboard("loader_run_updated")
		_ = c.UpdateLoaderLastError(ctx, loader.Summary.ID, run.Error)
		_ = c.AddLoaderEvent(ctx, loader.Summary.ID, run.ID, run.TriggerID, "loader.run.skipped", "warn", run.Error, nil, "", "", "")
		_ = c.WriteRunArtifact(run.ArtifactsDir, "error.txt", run.Error)
		return PreparedRun{Loader: loader, Trigger: trigger, Run: run, PayloadJSON: payloadJSON}, nil
	}

	if err := os.MkdirAll(run.ArtifactsDir, 0o755); err != nil {
		c.LeaveRun(loader.Summary.ID)
		return PreparedRun{}, fmt.Errorf("create loader run artifacts dir: %w", err)
	}
	_ = c.WriteRunArtifact(run.ArtifactsDir, "payload.json", payloadJSON)

	if err := c.CreateLoaderRun(ctx, run); err != nil {
		c.LeaveRun(loader.Summary.ID)
		return PreparedRun{}, err
	}
	c.UpdateTriggerEventDelivery(ctx, run)
	c.NotifyDashboard("loader_run_updated")
	_ = c.AddLoaderEvent(ctx, loader.Summary.ID, run.ID, run.TriggerID, "loader.run.started", "info", "loader run started", map[string]any{"source": run.TriggerSource}, "", "", "")
	return PreparedRun{Loader: loader, Trigger: trigger, Run: run, PayloadJSON: payloadJSON}, nil
}

func (e *RunExecutor) Execute(ctx context.Context, prepared PreparedRun) (LoaderRunSummary, error) {
	c := e.controller
	if prepared.Run.Status == LoaderRunStatusSkipped {
		return prepared.Run, nil
	}
	defer c.LeaveRun(prepared.Loader.Summary.ID)
	run := prepared.Run
	host := c.NewRunHost(prepared.Loader, &run, prepared.PayloadJSON)
	execution, execErr := c.ExecuteLoader(ctx, LoaderExecutionRequest{
		Runtime:     prepared.Loader.Summary.Runtime,
		Script:      prepared.Loader.Script,
		Trigger:     prepared.Trigger,
		PayloadJSON: prepared.PayloadJSON,
	}, host)

	writeCtx := context.WithoutCancel(ctx)
	c.CleanupRunHost(writeCtx, host)
	completedAt := time.Now().UTC()
	run.CompletedAt = completedAt
	run.DurationMs = completedAt.Sub(run.StartedAt).Milliseconds()
	if execErr != nil {
		run.Status = LoaderRunStatusFailed
		run.Error = execErr.Error()
		_ = c.WriteRunArtifact(run.ArtifactsDir, "error.txt", run.Error)
		_ = c.UpdateLoaderLastError(writeCtx, prepared.Loader.Summary.ID, run.Error)
		_ = c.AddLoaderEvent(writeCtx, prepared.Loader.Summary.ID, run.ID, run.TriggerID, "loader.run.failed", "error", run.Error, nil, "", "", "")
	} else {
		run.Status = LoaderRunStatusSucceeded
		run.ResultJSON = execution.ResultJSON
		if execution.ResultJSON != "" {
			_ = c.WriteRunArtifact(run.ArtifactsDir, "result.json", execution.ResultJSON)
		}
		_ = c.UpdateLoaderLastError(writeCtx, prepared.Loader.Summary.ID, "")
		_ = c.AddLoaderEvent(writeCtx, prepared.Loader.Summary.ID, run.ID, run.TriggerID, "loader.run.completed", "info", "loader run completed", map[string]any{"resultJson": execution.ResultJSON}, "", "", "")
	}
	if err := c.UpdateLoaderRun(writeCtx, run); err != nil {
		return LoaderRunSummary{}, err
	}
	c.UpdateTriggerEventDelivery(writeCtx, run)
	c.NotifyDashboard("loader_run_updated")
	if err := c.Refresh(writeCtx); err != nil {
		slog.Warn("failed to refresh loaders after run", "loader_id", prepared.Loader.Summary.ID, "error", err)
	}
	return run, nil
}

func (e *RunExecutor) Abort(ctx context.Context, prepared PreparedRun, reason string) {
	c := e.controller
	if prepared.Run.Status == LoaderRunStatusSkipped {
		return
	}
	defer c.LeaveRun(prepared.Loader.Summary.ID)
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "loader run aborted before execution"
	}
	run := prepared.Run
	completedAt := time.Now().UTC()
	run.Status = LoaderRunStatusFailed
	run.CompletedAt = completedAt
	run.DurationMs = completedAt.Sub(run.StartedAt).Milliseconds()
	run.Error = reason
	_ = c.WriteRunArtifact(run.ArtifactsDir, "error.txt", run.Error)
	_ = c.UpdateLoaderLastError(ctx, prepared.Loader.Summary.ID, run.Error)
	_ = c.AddLoaderEvent(ctx, prepared.Loader.Summary.ID, run.ID, run.TriggerID, "loader.run.failed", "error", run.Error, nil, "", "", "")
	if err := c.UpdateLoaderRun(ctx, run); err != nil {
		slog.Warn("failed to abort prepared loader run", "loader_id", prepared.Loader.Summary.ID, "run_id", run.ID, "error", err)
	}
	c.UpdateTriggerEventDelivery(ctx, run)
	c.NotifyDashboard("loader_run_updated")
}

func NormalizeJSONDocument(raw string) (string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return "", nil
	}
	var compact bytes.Buffer
	if err := json.Compact(&compact, []byte(raw)); err != nil {
		return "", fmt.Errorf("normalize json document: %w", err)
	}
	return compact.String(), nil
}
