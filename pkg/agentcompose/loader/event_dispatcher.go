package loader

import (
	"context"
	"errors"
	"log/slog"
	"strings"
	"time"
)

const TopicEventSourceWebhook = "webhook"

type EventTarget struct {
	Loader  Loader
	Trigger LoaderTrigger
}

type Reservation interface {
	Release()
}

type EventDispatcherController interface {
	RootContext() context.Context
	CollectTargets(topic string) []EventTarget
	ShouldRetryForBusy(event LoaderTopicEvent, targets []EventTarget) bool
	ReserveQueueSlots(event LoaderTopicEvent, count int) ([]Reservation, bool)
	LoaderRunTimeout(override time.Duration) time.Duration
	RunLoader(ctx context.Context, loader Loader, trigger *LoaderTrigger, payloadJSON, source string, automatic bool, options RunOptions, triggerEventAck ...func(context.Context) error) (LoaderRunSummary, error)
	EnterRun(loader Loader) bool
	LeaveRun(loaderID string)
	PrepareLoaderRun(ctx context.Context, loader Loader, trigger *LoaderTrigger, payloadJSON, source string, options RunOptions) (PreparedRun, error)
	ExecutePreparedLoaderRun(ctx context.Context, prepared PreparedRun) (LoaderRunSummary, error)
	AbortPreparedLoaderRun(ctx context.Context, prepared PreparedRun, reason string)
	RecordMatched(event LoaderTopicEvent, target EventTarget)
}

type EventDispatcher struct {
	controller EventDispatcherController
}

func NewEventDispatcher(controller EventDispatcherController) *EventDispatcher {
	return &EventDispatcher{controller: controller}
}

func (d *EventDispatcher) Dispatch(event LoaderTopicEvent) {
	payloadJSON, err := marshalJSONCompact(map[string]any{
		"topic":     event.Topic,
		"createdAt": event.CreatedAt.Format(time.RFC3339Nano),
		"payload":   event.Payload,
	})
	if err != nil {
		slog.Warn("failed to encode loader topic event payload", "topic", event.Topic, "error", err)
		return
	}
	targets := dedupeWebhookEventTargets(event, d.controller.CollectTargets(event.Topic))
	if len(targets) == 0 {
		d.ackNoSubscriber(event)
		return
	}
	if d.controller.ShouldRetryForBusy(event, targets) {
		d.retry(event, "loader is already running")
		return
	}
	reservations, ok := d.controller.ReserveQueueSlots(event, len(targets))
	if !ok {
		d.retry(event, "webhook queue is full")
		return
	}
	if event.Source == TopicEventSourceWebhook {
		d.dispatchWebhookTargets(event, targets, payloadJSON, reservations)
		return
	}
	d.dispatchTargets(event, targets, payloadJSON, reservations)
}

func (d *EventDispatcher) ackNoSubscriber(event LoaderTopicEvent) {
	ack := event.NoSubscriberAck
	if ack == nil {
		ack = event.Ack
	}
	if ack != nil {
		if err := ack(d.controller.RootContext()); err != nil {
			slog.Warn("failed to mark unmatched loader topic event published", "event_id", event.EventID, "topic", event.Topic, "error", err)
		}
	}
}

func (d *EventDispatcher) dispatchTargets(event LoaderTopicEvent, targets []EventTarget, payloadJSON string, reservations []Reservation) {
	for _, target := range targets {
		d.controller.RecordMatched(event, target)
		reservation := reservations[0]
		reservations = reservations[1:]
		runCtx, cancel := context.WithTimeout(d.controller.RootContext(), d.controller.LoaderRunTimeout(0))
		go func(target EventTarget, payloadJSON string, topic string, ack func(context.Context) error, release func(), reservation Reservation) {
			defer cancel()
			defer reservation.Release()
			if _, err := d.controller.RunLoader(runCtx, target.Loader, &target.Trigger, payloadJSON, topic, true, RunOptions{RetryWhenBusy: event.Source == TopicEventSourceWebhook}, ack); err != nil {
				if errors.Is(err, ErrRunBusyForRetry) {
					d.retry(event, "loader is already running")
					return
				}
				slog.Warn("loader event run failed", "loader_id", target.Loader.Summary.ID, "trigger_id", target.Trigger.ID, "topic", topic, "error", err)
				if release != nil {
					release()
				}
			}
		}(target, payloadJSON, event.Topic, event.Ack, event.Release, reservation)
	}
}

func (d *EventDispatcher) dispatchWebhookTargets(event LoaderTopicEvent, targets []EventTarget, payloadJSON string, reservations []Reservation) {
	acquiredLoaderIDs := make([]string, 0, len(targets))
	for _, target := range targets {
		if !d.controller.EnterRun(target.Loader) {
			for _, loaderID := range acquiredLoaderIDs {
				d.controller.LeaveRun(loaderID)
			}
			for _, reservation := range reservations {
				reservation.Release()
			}
			d.retry(event, "loader is already running")
			return
		}
		acquiredLoaderIDs = append(acquiredLoaderIDs, target.Loader.Summary.ID)
	}
	prepared := make([]PreparedRun, 0, len(targets))
	for index, target := range targets {
		preparedRun, err := d.controller.PrepareLoaderRun(d.controller.RootContext(), target.Loader, &target.Trigger, payloadJSON, event.Topic, RunOptions{AlreadyEntered: true})
		if err != nil {
			for _, item := range prepared {
				d.controller.AbortPreparedLoaderRun(context.WithoutCancel(d.controller.RootContext()), item, err.Error())
			}
			for _, loaderID := range acquiredLoaderIDs[index+1:] {
				d.controller.LeaveRun(loaderID)
			}
			for _, reservation := range reservations {
				reservation.Release()
			}
			reason := err.Error()
			if errors.Is(err, ErrRunBusyForRetry) {
				reason = "loader is already running"
			}
			d.retry(event, reason)
			return
		}
		prepared = append(prepared, preparedRun)
	}
	if event.Ack != nil {
		if err := event.Ack(d.controller.RootContext()); err != nil {
			slog.Warn("failed to mark loader topic event published", "event_id", event.EventID, "topic", event.Topic, "error", err)
		}
	}
	for index, item := range prepared {
		var reservation Reservation
		if index < len(reservations) {
			reservation = reservations[index]
		}
		runCtx, cancel := context.WithTimeout(d.controller.RootContext(), d.controller.LoaderRunTimeout(0))
		go func(item PreparedRun, reservation Reservation) {
			defer cancel()
			defer reservation.Release()
			if _, err := d.controller.ExecutePreparedLoaderRun(runCtx, item); err != nil {
				slog.Warn("loader event run failed", "loader_id", item.Loader.Summary.ID, "trigger_id", item.Run.TriggerID, "topic", event.Topic, "error", err)
			}
		}(item, reservation)
	}
}

func (d *EventDispatcher) retry(event LoaderTopicEvent, reason string) {
	reason = strings.TrimSpace(reason)
	if reason == "" {
		reason = "loader topic event retry requested"
	}
	if event.Retry != nil {
		if err := event.Retry(d.controller.RootContext(), reason, time.Now().UTC().Add(time.Second)); err != nil {
			slog.Warn("failed to retry loader topic event", "event_id", event.EventID, "topic", event.Topic, "reason", reason, "error", err)
		}
		return
	}
	if event.Release != nil {
		event.Release()
	}
}

func dedupeWebhookEventTargets(event LoaderTopicEvent, targets []EventTarget) []EventTarget {
	if event.Source != TopicEventSourceWebhook || len(targets) <= 1 {
		return targets
	}
	seen := map[string]struct{}{}
	deduped := make([]EventTarget, 0, len(targets))
	for _, target := range targets {
		loaderID := strings.TrimSpace(target.Loader.Summary.ID)
		if loaderID == "" {
			deduped = append(deduped, target)
			continue
		}
		if _, ok := seen[loaderID]; ok {
			continue
		}
		seen[loaderID] = struct{}{}
		deduped = append(deduped, target)
	}
	return deduped
}
