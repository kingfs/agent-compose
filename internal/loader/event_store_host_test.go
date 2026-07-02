package loader

import (
	"context"
	"encoding/json"
	"testing"
	"time"
)

func TestLoaderBusPublishReportsFullChannel(t *testing.T) {
	bus := &LoaderBus{ch: make(chan LoaderTopicEvent, 1)}
	if !bus.Publish(LoaderTopicEvent{Topic: "webhook.test", Payload: map[string]any{}, CreatedAt: time.Now().UTC()}) {
		t.Fatalf("first Publish returned false, want true")
	}
	if bus.Publish(LoaderTopicEvent{Topic: "webhook.test", Payload: map[string]any{}, CreatedAt: time.Now().UTC()}) {
		t.Fatalf("second Publish returned true for full channel")
	}
	if bus.Publish(LoaderTopicEvent{}) {
		t.Fatalf("Publish with empty topic returned true")
	}
}

func TestLoaderRunHostPublishEventStoresDerivedEvent(t *testing.T) {
	ctx := context.Background()
	store := newTestConfigStore(t)
	manager := &LoaderManager{configDB: store}
	host := &loaderRunHost{
		manager: manager,
		loader:  Loader{Summary: LoaderSummary{ID: "loader-1"}},
		run:     &LoaderRunSummary{ID: "run-1", LoaderID: "loader-1", TriggerID: "trigger-1"},
		triggerEvent: loaderTriggerEventMetadata{
			EventID:       "evt-parent",
			CorrelationID: "corr-parent",
			Sequence:      10,
		},
	}

	created, err := host.PublishEvent(ctx, "runtime.test.requested", `{"provider":"test-runtime","value":1}`)
	if err != nil {
		t.Fatalf("PublishEvent returned error: %v", err)
	}
	if created.CorrelationID != "corr-parent" || created.ParentEventID != "evt-parent" {
		t.Fatalf("created event inheritance = %#v", created)
	}
	if created.PublisherID != "loader-1" || created.PublisherRunID != "run-1" {
		t.Fatalf("publisher metadata = %#v", created)
	}
	if created.Provider != "test-runtime" {
		t.Fatalf("provider = %q, want test-runtime", created.Provider)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(created.PayloadJSON), &envelope); err != nil {
		t.Fatalf("decode envelope: %v", err)
	}
	if envelope["sequence"].(float64) != float64(created.Sequence) {
		t.Fatalf("sequence in envelope = %#v, want %d", envelope["sequence"], created.Sequence)
	}

	if _, err := host.PublishEvent(ctx, "webhook.not.allowed", `{}`); err == nil {
		t.Fatalf("PublishEvent with webhook topic returned nil error")
	}
}

func TestLoaderRunHostLinkedLoaderEventStoresEventSessionLink(t *testing.T) {
	ctx := context.Background()
	store := newTestConfigStore(t)
	manager := &LoaderManager{configDB: store}
	loader := createTestLoader(t, ctx, store)
	run := LoaderRunSummary{
		ID:            "run-link",
		LoaderID:      loader.Summary.ID,
		TriggerID:     "trigger-link",
		TriggerKind:   LoaderTriggerKindEvent,
		TriggerSource: "event",
		Status:        LoaderRunStatusRunning,
		StartedAt:     time.Now().UTC(),
	}
	if err := store.CreateLoaderRun(ctx, run); err != nil {
		t.Fatalf("CreateLoaderRun returned error: %v", err)
	}
	host := &loaderRunHost{
		manager: manager,
		loader:  loader,
		run:     &run,
		triggerEvent: loaderTriggerEventMetadata{
			EventID: "evt-link",
		},
	}

	if err := host.addLinkedLoaderEvent(ctx, "loader.command.completed", "info", "command completed", map[string]any{"sessionId": "session-link"}, "session-link", "cell-link", ""); err != nil {
		t.Fatalf("addLinkedLoaderEvent returned error: %v", err)
	}
	links, err := store.ListEventSessionLinks(ctx, []string{"evt-link"})
	if err != nil {
		t.Fatalf("ListEventSessionLinks returned error: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("event session links = %#v, want one link", links)
	}
	link := links[0]
	if link.EventID != "evt-link" || link.SessionID != "session-link" || link.Relation != "loader.command.completed" {
		t.Fatalf("event session link identity = %#v", link)
	}
	if link.LoaderID != loader.Summary.ID || link.RunID != "run-link" || link.TriggerID != "trigger-link" || link.LoaderEventID == "" {
		t.Fatalf("event session link metadata = %#v", link)
	}

	noEventRun := LoaderRunSummary{
		ID:            "run-no-event",
		LoaderID:      loader.Summary.ID,
		TriggerID:     "trigger-link",
		TriggerKind:   LoaderTriggerKindEvent,
		TriggerSource: "event",
		Status:        LoaderRunStatusRunning,
		StartedAt:     time.Now().UTC(),
	}
	if err := store.CreateLoaderRun(ctx, noEventRun); err != nil {
		t.Fatalf("CreateLoaderRun without trigger event returned error: %v", err)
	}
	noEventHost := &loaderRunHost{
		manager: manager,
		loader:  loader,
		run:     &noEventRun,
	}
	if err := noEventHost.addLinkedLoaderEvent(ctx, "loader.command.completed", "info", "command completed", nil, "session-no-event", "", ""); err != nil {
		t.Fatalf("addLinkedLoaderEvent without trigger event returned error: %v", err)
	}
	links, err = store.ListEventSessionLinks(ctx, []string{"evt-link"})
	if err != nil {
		t.Fatalf("ListEventSessionLinks after no-event run returned error: %v", err)
	}
	if len(links) != 1 {
		t.Fatalf("event session links after no-event run = %#v, want original link only", links)
	}
}
