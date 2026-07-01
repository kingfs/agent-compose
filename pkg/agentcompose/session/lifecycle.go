package session

import (
	"context"
	"fmt"
	"time"

	appconfig "agent-compose/pkg/config"
	driverpkg "agent-compose/pkg/driver"

	"github.com/google/uuid"
)

type LifecycleStore interface {
	CreateSession(context.Context, string, string, string, string, string, string, *SessionWorkspace, []SessionEnvVar, []SessionTag) (*Session, error)
	GetSession(context.Context, string) (*Session, error)
	ListSessions(context.Context, SessionListOptions) (SessionListResult, error)
	UpdateSession(context.Context, *Session) error
	AddEvent(context.Context, string, SessionEvent) error
	GetVMState(string) (VMState, error)
	SaveVMState(string, VMState) error
	GetProxyState(string) (ProxyState, error)
}

type LifecycleDriver interface {
	StartSessionVM(context.Context, *Session) error
	StopSessionVM(context.Context, *Session) error
}

type AliveRuntime interface {
	IsSessionAlive(context.Context, *Session, VMState) (bool, error)
}

type StreamPublisher interface {
	PublishSessionUpdated(*SessionSummary)
	PublishEventAdded(string, SessionEvent)
}

type LLMTokenRevoker interface {
	RevokeLLMFacadeTokensForSession(context.Context, string) error
}

type LifecycleHooks struct {
	PrepareWorkspace func(context.Context, *Session) error
	WriteCapability  func(context.Context, *Session, []string)
	PublishTopic     func(string, *Session, string)
	NotifyDashboard  func(string)
	JupyterReachable func(ProxyState, time.Duration) bool
	IsSessionAlive   func(context.Context, string, *Session, VMState) (bool, bool, error)
}

type Lifecycle struct {
	config  *appconfig.Config
	store   LifecycleStore
	driver  LifecycleDriver
	streams StreamPublisher
	revoker LLMTokenRevoker
	hooks   LifecycleHooks
}

func NewLifecycle(config *appconfig.Config, store LifecycleStore, driver LifecycleDriver, streams StreamPublisher, revoker LLMTokenRevoker, hooks LifecycleHooks) *Lifecycle {
	return &Lifecycle{config: config, store: store, driver: driver, streams: streams, revoker: revoker, hooks: hooks}
}

type CreateRequest struct {
	Title            string
	BaseWorkspace    string
	Driver           string
	GuestImage       string
	WorkspaceID      string
	Source           string
	Workspace        *SessionWorkspace
	EnvItems         []SessionEnvVar
	ProviderEnvItems []SessionEnvVar
	Tags             []SessionTag
	CapsetIDs        []string
}

func (l *Lifecycle) Create(ctx context.Context, req CreateRequest) (*Session, error) {
	item, err := l.store.CreateSession(ctx, req.Title, req.BaseWorkspace, req.Driver, req.GuestImage, req.WorkspaceID, req.Source, req.Workspace, req.EnvItems, req.Tags)
	if err != nil {
		return nil, err
	}
	item.ProviderEnvItems = req.ProviderEnvItems
	if err := l.prepareWorkspace(ctx, item); err != nil {
		item.Summary.VMStatus = VMStatusFailed
		_ = l.store.UpdateSession(ctx, item)
		return nil, err
	}
	l.writeCapability(ctx, item, req.CapsetIDs)
	if err := l.driver.StartSessionVM(ctx, item); err != nil {
		item.Summary.VMStatus = VMStatusFailed
		_ = l.store.UpdateSession(ctx, item)
		return nil, err
	}
	item.Summary.VMStatus = VMStatusRunning
	if err := l.store.UpdateSession(ctx, item); err != nil {
		return nil, err
	}
	l.publishSessionUpdated(&item.Summary)
	l.notifyDashboard("session_updated")
	event := SessionEvent{
		ID:        uuid.NewString(),
		Type:      "session.created",
		Level:     "info",
		Message:   fmt.Sprintf("session started with %s driver using guest image %s", item.Summary.Driver, item.Summary.GuestImage),
		CreatedAt: time.Now().UTC(),
	}
	_ = l.store.AddEvent(ctx, item.Summary.ID, event)
	l.publishEventAdded(item.Summary.ID, event)
	loaded, err := l.store.GetSession(ctx, item.Summary.ID)
	if err != nil {
		return nil, err
	}
	RestoreTransientFields(loaded, item)
	l.publishTopic("agent-compose.session.created", loaded, req.Source)
	return loaded, nil
}

func (l *Lifecycle) Resume(ctx context.Context, sessionID, source string, capsetIDs []string) (*Session, error) {
	item, err := l.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if err := l.prepareWorkspace(ctx, item); err != nil {
		return nil, err
	}
	l.writeCapability(ctx, item, capsetIDs)
	if err := l.driver.StartSessionVM(ctx, item); err != nil {
		return nil, err
	}
	item.Summary.VMStatus = VMStatusRunning
	if err := l.store.UpdateSession(ctx, item); err != nil {
		return nil, err
	}
	l.publishSessionUpdated(&item.Summary)
	l.notifyDashboard("session_updated")
	event := SessionEvent{ID: uuid.NewString(), Type: "session.resumed", Level: "info", Message: fmt.Sprintf("session resumed with %s driver using guest image %s", item.Summary.Driver, item.Summary.GuestImage), CreatedAt: time.Now().UTC()}
	_ = l.store.AddEvent(ctx, item.Summary.ID, event)
	l.publishEventAdded(item.Summary.ID, event)
	loaded, err := l.store.GetSession(ctx, item.Summary.ID)
	if err != nil {
		return nil, err
	}
	RestoreTransientFields(loaded, item)
	l.publishTopic("agent-compose.session.resumed", loaded, source)
	return loaded, nil
}

func (l *Lifecycle) Stop(ctx context.Context, sessionID, source string) (*Session, error) {
	item, err := l.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if reconciled, err := l.ReconcileRuntimeState(ctx, item); err == nil {
		item = reconciled
	}
	if item.Summary.VMStatus != VMStatusRunning {
		return item, nil
	}
	if err := l.driver.StopSessionVM(ctx, item); err != nil {
		return nil, err
	}
	item.Summary.VMStatus = VMStatusStopped
	if err := l.store.UpdateSession(ctx, item); err != nil {
		return nil, err
	}
	l.publishSessionUpdated(&item.Summary)
	l.notifyDashboard("session_updated")
	event := SessionEvent{ID: uuid.NewString(), Type: "session.stopped", Level: "info", Message: "session stopped", CreatedAt: time.Now().UTC()}
	_ = l.store.AddEvent(ctx, item.Summary.ID, event)
	l.publishEventAdded(item.Summary.ID, event)
	loaded, err := l.store.GetSession(ctx, item.Summary.ID)
	if err != nil {
		return nil, err
	}
	l.publishTopic("agent-compose.session.stopped", loaded, source)
	return loaded, nil
}

func (l *Lifecycle) Get(ctx context.Context, sessionID string) (*Session, error) {
	item, err := l.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, err
	}
	if reconciled, err := l.ReconcileRuntimeState(ctx, item); err == nil {
		item = reconciled
	}
	return item, nil
}

func (l *Lifecycle) List(ctx context.Context, options SessionListOptions) (SessionListResult, error) {
	result, err := l.store.ListSessions(ctx, options)
	if err != nil {
		return SessionListResult{}, err
	}
	for index, item := range result.Sessions {
		if reconciled, err := l.ReconcileRuntimeState(ctx, item); err == nil {
			result.Sessions[index] = reconciled
		}
	}
	return result, nil
}

func (l *Lifecycle) EnsureProxyReady(ctx context.Context, sessionID string) (*Session, ProxyState, error) {
	item, err := l.store.GetSession(ctx, sessionID)
	if err != nil {
		return nil, ProxyState{}, err
	}
	proxyState, err := l.store.GetProxyState(item.Summary.ID)
	if err != nil {
		return nil, ProxyState{}, err
	}
	if item.Summary.VMStatus == VMStatusRunning && l.jupyterReachable(proxyState, 1500*time.Millisecond) {
		return item, proxyState, nil
	}
	startCtx, cancel := context.WithTimeout(ctx, l.config.SessionStartTimeout)
	defer cancel()
	if err := l.prepareWorkspace(startCtx, item); err != nil {
		item.Summary.VMStatus = VMStatusFailed
		_ = l.store.UpdateSession(ctx, item)
		return nil, ProxyState{}, err
	}
	if err := l.driver.StartSessionVM(startCtx, item); err != nil {
		item.Summary.VMStatus = VMStatusFailed
		_ = l.store.UpdateSession(ctx, item)
		return nil, ProxyState{}, err
	}
	item.Summary.VMStatus = VMStatusRunning
	if err := l.store.UpdateSession(ctx, item); err != nil {
		return nil, ProxyState{}, err
	}
	loaded, err := l.store.GetSession(ctx, item.Summary.ID)
	if err != nil {
		return nil, ProxyState{}, err
	}
	proxyState, err = l.store.GetProxyState(item.Summary.ID)
	if err != nil {
		return nil, ProxyState{}, err
	}
	return loaded, proxyState, nil
}

func (l *Lifecycle) ReconcileRuntimeState(ctx context.Context, item *Session) (*Session, error) {
	if item == nil || item.Summary.VMStatus != VMStatusRunning {
		return item, nil
	}
	driver, err := driverpkg.ResolveSessionRuntimeDriver(item.Summary.Driver, l.config.RuntimeDriver)
	if err != nil {
		return nil, err
	}
	if driver != driverpkg.RuntimeDriverMicrosandbox {
		return item, nil
	}
	proxyState, err := l.store.GetProxyState(item.Summary.ID)
	if err != nil {
		return nil, err
	}
	if l.jupyterReachable(proxyState, 250*time.Millisecond) {
		return item, nil
	}
	vmState, err := l.store.GetVMState(item.Summary.ID)
	if err != nil {
		return nil, err
	}
	alive, supported, err := l.isSessionAlive(ctx, driver, item, vmState)
	if err != nil {
		return nil, err
	}
	if !supported {
		return item, nil
	}
	if alive {
		return item, nil
	}
	now := time.Now().UTC()
	vmState.StoppedAt = now
	vmState.LastError = ""
	vmState.BoxID = ""
	if err := l.store.SaveVMState(item.Summary.ID, vmState); err != nil {
		return nil, err
	}
	item.Summary.VMStatus = VMStatusStopped
	if err := l.store.UpdateSession(ctx, item); err != nil {
		return nil, err
	}
	if l.revoker != nil {
		_ = l.revoker.RevokeLLMFacadeTokensForSession(ctx, item.Summary.ID)
	}
	l.publishSessionUpdated(&item.Summary)
	l.notifyDashboard("session_updated")
	event := SessionEvent{
		ID:        uuid.NewString(),
		Type:      "session.runtime_lost",
		Level:     "warn",
		Message:   "session marked stopped after microsandbox runtime became unreachable",
		CreatedAt: now,
	}
	_ = l.store.AddEvent(ctx, item.Summary.ID, event)
	l.publishEventAdded(item.Summary.ID, event)
	return l.store.GetSession(ctx, item.Summary.ID)
}

func (l *Lifecycle) prepareWorkspace(ctx context.Context, item *Session) error {
	if l.hooks.PrepareWorkspace == nil {
		return nil
	}
	return l.hooks.PrepareWorkspace(ctx, item)
}

func (l *Lifecycle) writeCapability(ctx context.Context, item *Session, capsetIDs []string) {
	if l.hooks.WriteCapability != nil {
		l.hooks.WriteCapability(ctx, item, capsetIDs)
	}
}

func (l *Lifecycle) publishTopic(topic string, item *Session, source string) {
	if l.hooks.PublishTopic != nil {
		l.hooks.PublishTopic(topic, item, source)
	}
}

func (l *Lifecycle) notifyDashboard(reason string) {
	if l.hooks.NotifyDashboard != nil {
		l.hooks.NotifyDashboard(reason)
	}
}

func (l *Lifecycle) jupyterReachable(proxyState ProxyState, timeout time.Duration) bool {
	if l.hooks.JupyterReachable == nil {
		return false
	}
	return l.hooks.JupyterReachable(proxyState, timeout)
}

func (l *Lifecycle) isSessionAlive(ctx context.Context, driver string, item *Session, vmState VMState) (bool, bool, error) {
	if l.hooks.IsSessionAlive == nil {
		return false, false, nil
	}
	return l.hooks.IsSessionAlive(ctx, driver, item, vmState)
}

func (l *Lifecycle) publishSessionUpdated(summary *SessionSummary) {
	if l.streams != nil {
		l.streams.PublishSessionUpdated(summary)
	}
}

func (l *Lifecycle) publishEventAdded(sessionID string, event SessionEvent) {
	if l.streams != nil {
		l.streams.PublishEventAdded(sessionID, event)
	}
}
