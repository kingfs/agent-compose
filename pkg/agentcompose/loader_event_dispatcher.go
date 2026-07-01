package agentcompose

import (
	"context"
	"log/slog"
	"strings"
	"time"

	loaderpkg "agent-compose/pkg/agentcompose/loader"
)

type LoaderEventDispatcher struct {
	manager *LoaderManager
	inner   *loaderpkg.EventDispatcher
}

func NewLoaderEventDispatcher(manager *LoaderManager) *LoaderEventDispatcher {
	dispatcher := &LoaderEventDispatcher{manager: manager}
	dispatcher.inner = loaderpkg.NewEventDispatcher(loaderEventController{dispatcher: dispatcher})
	return dispatcher
}

func (d *LoaderEventDispatcher) Dispatch(event LoaderTopicEvent) {
	d.inner.Dispatch(event)
}

func (d *LoaderEventDispatcher) reserveQueueSlots(event LoaderTopicEvent, count int) ([]*webhookQueueReservation, bool) {
	m := d.manager
	if count <= 0 {
		return nil, true
	}
	if event.Source != TopicEventSourceWebhook {
		return noopWebhookQueueReservations(count), true
	}
	if m.eventQueue == nil {
		queue, err := newWebhookRunQueueFromConfig(m.config)
		if err != nil {
			slog.Warn("failed to initialize webhook queue config", "error", err)
			queue = &WebhookRunQueue{running: map[string]int{}}
		}
		m.eventQueue = queue
	}
	reservations := make([]*webhookQueueReservation, 0, count)
	for i := 0; i < count; i++ {
		reservation, ok := m.eventQueue.Reserve(event)
		if !ok {
			for _, reserved := range reservations {
				reserved.Release()
			}
			return nil, false
		}
		reservations = append(reservations, reservation)
	}
	return reservations, true
}

type loaderEventController struct {
	dispatcher *LoaderEventDispatcher
}

func (c loaderEventController) RootContext() context.Context {
	return c.dispatcher.manager.rootCtx
}

func (c loaderEventController) CollectTargets(topic string) []loaderpkg.EventTarget {
	targets := make([]loaderpkg.EventTarget, 0)
	for _, loader := range c.dispatcher.manager.snapshotLoaders() {
		if !loader.Summary.Enabled {
			continue
		}
		for _, trigger := range loader.Triggers {
			if !trigger.Enabled || trigger.Kind != LoaderTriggerKindEvent || !loaderTriggerTopicMatches(trigger.Topic, topic) {
				continue
			}
			targets = append(targets, loaderpkg.EventTarget{
				Loader:  loader,
				Trigger: trigger,
			})
		}
	}
	return targets
}

func (c loaderEventController) ShouldRetryForBusy(event LoaderTopicEvent, targets []loaderpkg.EventTarget) bool {
	if event.Source != TopicEventSourceWebhook || len(targets) == 0 {
		return false
	}
	m := c.dispatcher.manager
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, target := range targets {
		loaderID := strings.TrimSpace(target.Loader.Summary.ID)
		if normalizeLoaderConcurrencyPolicy(target.Loader.Summary.ConcurrencyPolicy) != LoaderConcurrencyPolicyParallel && m.running[loaderID] > 0 {
			return true
		}
	}
	return false
}

func (c loaderEventController) ReserveQueueSlots(event LoaderTopicEvent, count int) ([]loaderpkg.Reservation, bool) {
	reservations, ok := c.dispatcher.reserveQueueSlots(event, count)
	if !ok {
		return nil, false
	}
	items := make([]loaderpkg.Reservation, 0, len(reservations))
	for _, reservation := range reservations {
		items = append(items, reservation)
	}
	return items, true
}

func (c loaderEventController) LoaderRunTimeout(override time.Duration) time.Duration {
	return c.dispatcher.manager.loaderRunTimeout(override)
}

func (c loaderEventController) RunLoader(ctx context.Context, loader Loader, trigger *LoaderTrigger, payloadJSON, source string, automatic bool, options loaderpkg.RunOptions, triggerEventAck ...func(context.Context) error) (LoaderRunSummary, error) {
	return c.dispatcher.manager.runLoader(ctx, loader, trigger, payloadJSON, source, automatic, fromModuleRunOptions(options), triggerEventAck...)
}

func (c loaderEventController) EnterRun(loader Loader) bool {
	return c.dispatcher.manager.enterRun(loader)
}

func (c loaderEventController) LeaveRun(loaderID string) {
	c.dispatcher.manager.leaveRun(loaderID)
}

func (c loaderEventController) PrepareLoaderRun(ctx context.Context, loader Loader, trigger *LoaderTrigger, payloadJSON, source string, options loaderpkg.RunOptions) (loaderpkg.PreparedRun, error) {
	prepared, err := c.dispatcher.manager.prepareLoaderRun(ctx, loader, trigger, payloadJSON, source, fromModuleRunOptions(options))
	return toModulePreparedRun(prepared), err
}

func (c loaderEventController) ExecutePreparedLoaderRun(ctx context.Context, prepared loaderpkg.PreparedRun) (LoaderRunSummary, error) {
	return c.dispatcher.manager.executePreparedLoaderRun(ctx, fromModulePreparedRun(prepared))
}

func (c loaderEventController) AbortPreparedLoaderRun(ctx context.Context, prepared loaderpkg.PreparedRun, reason string) {
	c.dispatcher.manager.abortPreparedLoaderRun(ctx, fromModulePreparedRun(prepared), reason)
}

func (c loaderEventController) RecordMatched(event LoaderTopicEvent, target loaderpkg.EventTarget) {
	if event.EventID == "" {
		return
	}
	if err := c.dispatcher.manager.configDB.UpsertEventDelivery(c.dispatcher.manager.rootCtx, EventDelivery{
		EventID:   event.EventID,
		LoaderID:  target.Loader.Summary.ID,
		TriggerID: target.Trigger.ID,
		Status:    EventDeliveryStatusMatched,
	}); err != nil {
		slog.Warn("failed to record event delivery match", "event_id", event.EventID, "loader_id", target.Loader.Summary.ID, "trigger_id", target.Trigger.ID, "error", err)
	}
}

func fromModuleRunOptions(options loaderpkg.RunOptions) loaderRunOptions {
	return loaderRunOptions{alreadyEntered: options.AlreadyEntered, retryWhenBusy: options.RetryWhenBusy}
}
