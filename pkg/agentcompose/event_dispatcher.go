package agentcompose

import (
	"context"
	"time"

	eventmodule "agent-compose/pkg/agentcompose/event"
)

type EventDispatcher struct {
	inner    *eventmodule.Dispatcher
	interval time.Duration
}

func NewEventDispatcher(rootCtx context.Context, configDB *ConfigStore, bus *LoaderBus) *EventDispatcher {
	return &EventDispatcher{
		inner:    eventmodule.NewDispatcher(rootCtx, configDB, loaderEventBusAdapter{bus: bus}),
		interval: 500 * time.Millisecond,
	}
}

func (d *EventDispatcher) Start() {
	if d == nil || d.inner == nil {
		return
	}
	d.inner.SetInterval(d.interval)
	d.inner.Start()
}

func (d *EventDispatcher) dispatchOnce(ctx context.Context, limit int) {
	if d == nil || d.inner == nil {
		return
	}
	d.inner.DispatchOnce(ctx, limit)
}

type loaderEventBusAdapter struct {
	bus *LoaderBus
}

func (a loaderEventBusAdapter) Publish(item eventmodule.BusEvent) bool {
	if a.bus == nil {
		return false
	}
	return a.bus.Publish(LoaderTopicEvent{
		EventID:         item.EventID,
		Topic:           item.Topic,
		Source:          item.Source,
		Provider:        item.Provider,
		Payload:         item.Payload,
		CreatedAt:       item.CreatedAt,
		Ack:             item.Ack,
		NoSubscriberAck: item.NoSubscriberAck,
		Retry: func(ctx context.Context, reason string, nextAttemptAt time.Time) error {
			return item.Retry(ctx, reason, nextAttemptAt)
		},
		Release: item.Release,
	})
}
