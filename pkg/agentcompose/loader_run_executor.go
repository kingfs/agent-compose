package agentcompose

import (
	"context"
	"errors"

	loaderpkg "agent-compose/pkg/agentcompose/loader"
)

type LoaderRunExecutor struct {
	manager *LoaderManager
	inner   *loaderpkg.RunExecutor
}

type loaderRunOptions struct {
	alreadyEntered bool
	retryWhenBusy  bool
}

var errLoaderRunBusyForRetry = loaderpkg.ErrRunBusyForRetry

func NewLoaderRunExecutor(manager *LoaderManager) *LoaderRunExecutor {
	executor := &LoaderRunExecutor{manager: manager}
	executor.inner = loaderpkg.NewRunExecutor(loaderRunController{manager: manager})
	return executor
}

func (e *LoaderRunExecutor) Run(ctx context.Context, loader Loader, trigger *LoaderTrigger, payloadJSON, source string, options loaderRunOptions, triggerEventAck ...func(context.Context) error) (LoaderRunSummary, error) {
	return e.inner.Run(ctx, loader, trigger, payloadJSON, source, toModuleRunOptions(options), triggerEventAck...)
}

func (e *LoaderRunExecutor) Prepare(ctx context.Context, loader Loader, trigger *LoaderTrigger, payloadJSON, source string, options loaderRunOptions) (preparedLoaderRun, error) {
	prepared, err := e.inner.Prepare(ctx, loader, trigger, payloadJSON, source, toModuleRunOptions(options))
	return fromModulePreparedRun(prepared), err
}

func (e *LoaderRunExecutor) Execute(ctx context.Context, prepared preparedLoaderRun) (LoaderRunSummary, error) {
	return e.inner.Execute(ctx, toModulePreparedRun(prepared))
}

func (e *LoaderRunExecutor) Abort(ctx context.Context, prepared preparedLoaderRun, reason string) {
	e.inner.Abort(ctx, toModulePreparedRun(prepared), reason)
}

type loaderRunController struct {
	manager *LoaderManager
}

func (c loaderRunController) EnterRun(loader Loader) bool {
	return c.manager.enterRun(loader)
}

func (c loaderRunController) LeaveRun(loaderID string) {
	c.manager.leaveRun(loaderID)
}

func (c loaderRunController) RunArtifactsDir(loaderID, runID string) string {
	return c.manager.runArtifactsDir(loaderID, runID)
}

func (c loaderRunController) WriteRunArtifact(dir, name, content string) error {
	return c.manager.writeRunArtifact(dir, name, content)
}

func (c loaderRunController) CreateLoaderRun(ctx context.Context, run LoaderRunSummary) error {
	return c.manager.configDB.CreateLoaderRun(ctx, run)
}

func (c loaderRunController) UpdateLoaderRun(ctx context.Context, run LoaderRunSummary) error {
	return c.manager.configDB.UpdateLoaderRun(ctx, run)
}

func (c loaderRunController) UpdateLoaderLastError(ctx context.Context, loaderID, lastError string) error {
	return c.manager.configDB.UpdateLoaderLastError(ctx, loaderID, lastError)
}

func (c loaderRunController) UpdateTriggerEventDelivery(ctx context.Context, run LoaderRunSummary) {
	c.manager.updateTriggerEventDelivery(ctx, run)
}

func (c loaderRunController) NotifyDashboard(reason string) {
	c.manager.notifyDashboard(reason)
}

func (c loaderRunController) AddLoaderEvent(ctx context.Context, loaderID, runID, triggerID, eventType, level, message string, payload any, linkedSessionID, linkedCellID, linkedAgentSessionID string) error {
	return c.manager.addLoaderEvent(ctx, loaderID, runID, triggerID, eventType, level, message, payload, linkedSessionID, linkedCellID, linkedAgentSessionID)
}

func (c loaderRunController) Refresh(ctx context.Context) error {
	return c.manager.Refresh(ctx)
}

func (c loaderRunController) ExecuteLoader(ctx context.Context, request LoaderExecutionRequest, host LoaderHost) (LoaderExecutionResult, error) {
	return c.manager.engine.Execute(ctx, request, host)
}

func (c loaderRunController) NewRunHost(loader Loader, run *LoaderRunSummary, payloadJSON string) loaderpkg.LoaderHost {
	return &loaderRunHost{manager: c.manager, loader: loader, run: run, triggerEvent: parseLoaderTriggerEventMetadata(payloadJSON)}
}

func (c loaderRunController) CleanupRunHost(ctx context.Context, host loaderpkg.LoaderHost) {
	if h, ok := host.(*loaderRunHost); ok {
		h.cleanupCommandSessions(ctx)
	}
}

func toModuleRunOptions(options loaderRunOptions) loaderpkg.RunOptions {
	return loaderpkg.RunOptions{AlreadyEntered: options.alreadyEntered, RetryWhenBusy: options.retryWhenBusy}
}

func toModulePreparedRun(prepared preparedLoaderRun) loaderpkg.PreparedRun {
	return loaderpkg.PreparedRun{Loader: prepared.loader, Trigger: prepared.trigger, Run: prepared.run, PayloadJSON: prepared.payloadJSON}
}

func fromModulePreparedRun(prepared loaderpkg.PreparedRun) preparedLoaderRun {
	return preparedLoaderRun{loader: prepared.Loader, trigger: prepared.Trigger, run: prepared.Run, payloadJSON: prepared.PayloadJSON}
}

func (m *LoaderManager) runExecutorComponent() *LoaderRunExecutor {
	m.initLoaderComponents()
	return m.runExecutor
}

func (m *LoaderManager) runLoader(ctx context.Context, loader Loader, trigger *LoaderTrigger, payloadJSON, source string, automatic bool, options loaderRunOptions, triggerEventAck ...func(context.Context) error) (LoaderRunSummary, error) {
	run, err := m.runExecutorComponent().Run(ctx, loader, trigger, payloadJSON, source, options, triggerEventAck...)
	if errors.Is(err, loaderpkg.ErrRunBusyForRetry) {
		return run, errLoaderRunBusyForRetry
	}
	return run, err
}

func (m *LoaderManager) prepareLoaderRun(ctx context.Context, loader Loader, trigger *LoaderTrigger, payloadJSON, source string, options loaderRunOptions) (preparedLoaderRun, error) {
	prepared, err := m.runExecutorComponent().Prepare(ctx, loader, trigger, payloadJSON, source, options)
	if errors.Is(err, loaderpkg.ErrRunBusyForRetry) {
		return prepared, errLoaderRunBusyForRetry
	}
	return prepared, err
}

func (m *LoaderManager) executePreparedLoaderRun(ctx context.Context, prepared preparedLoaderRun) (LoaderRunSummary, error) {
	return m.runExecutorComponent().Execute(ctx, prepared)
}

func (m *LoaderManager) abortPreparedLoaderRun(ctx context.Context, prepared preparedLoaderRun, reason string) {
	m.runExecutorComponent().Abort(ctx, prepared, reason)
}
