package loader

import (
	"reflect"
	"strings"
	"testing"
	"time"
)

func TestNormalizeLoaderAppliesModuleDefaults(t *testing.T) {
	item, err := Normalize(Loader{
		Summary: LoaderSummary{
			ID:                " loader-1 ",
			Runtime:           " ",
			DefaultAgent:      " claude-code ",
			SessionPolicy:     " reuse ",
			ConcurrencyPolicy: " allow ",
			CapsetIDs:         []string{" cap-b ", "cap-a", "cap-b", " "},
		},
		Script: "console.log('ok')\r\n",
		EnvItems: []SessionEnvVar{
			{Name: " B ", Value: "2"},
			{Name: "A", Value: "1"},
			{Name: "B", Value: "latest"},
			{Name: " ", Value: "ignored"},
		},
	}, false)
	if err != nil {
		t.Fatalf("Normalize returned error: %v", err)
	}

	if item.Summary.ID != "loader-1" {
		t.Fatalf("id = %q", item.Summary.ID)
	}
	if item.Summary.Runtime != LoaderRuntimeScheduler {
		t.Fatalf("runtime = %q", item.Summary.Runtime)
	}
	if item.Summary.DefaultAgent != "claude" {
		t.Fatalf("default agent = %q", item.Summary.DefaultAgent)
	}
	if item.Summary.SessionPolicy != LoaderSessionPolicySticky {
		t.Fatalf("session policy = %q", item.Summary.SessionPolicy)
	}
	if item.Summary.ConcurrencyPolicy != LoaderConcurrencyPolicyParallel {
		t.Fatalf("concurrency policy = %q", item.Summary.ConcurrencyPolicy)
	}
	if strings.Contains(item.Script, "\r\n") {
		t.Fatalf("script still contains CRLF: %q", item.Script)
	}
	if !reflect.DeepEqual(item.Summary.CapsetIDs, []string{"cap-b", "cap-a"}) {
		t.Fatalf("capset ids = %#v", item.Summary.CapsetIDs)
	}
	if !reflect.DeepEqual(item.EnvItems, []SessionEnvVar{{Name: "A", Value: "1"}, {Name: "B", Value: "latest"}}) {
		t.Fatalf("env items = %#v", item.EnvItems)
	}
}

func TestNormalizeTriggerCanonicalizesScheduleFields(t *testing.T) {
	next := time.Date(2026, 7, 2, 9, 30, 0, 0, time.FixedZone("CST", 8*60*60))
	last := time.Date(2026, 7, 2, 8, 30, 0, 0, time.FixedZone("CST", 8*60*60))

	trigger, err := NormalizeTrigger(" loader-1 ", LoaderTrigger{
		ID:          " trigger-1 ",
		Kind:        " INTERVAL ",
		Topic:       "ignored",
		IntervalMs:  1500,
		SpecJSON:    " ",
		NextFireAt:  next,
		LastFiredAt: last,
	})
	if err != nil {
		t.Fatalf("NormalizeTrigger returned error: %v", err)
	}

	if trigger.LoaderID != "loader-1" || trigger.ID != "trigger-1" {
		t.Fatalf("ids = %q %q", trigger.LoaderID, trigger.ID)
	}
	if trigger.Kind != LoaderTriggerKindInterval {
		t.Fatalf("kind = %q", trigger.Kind)
	}
	if trigger.Topic != "" {
		t.Fatalf("topic = %q", trigger.Topic)
	}
	if trigger.SpecJSON != "{}" {
		t.Fatalf("spec json = %q", trigger.SpecJSON)
	}
	if trigger.NextFireAt.Location() != time.UTC || !trigger.NextFireAt.Equal(next.UTC()) {
		t.Fatalf("next fire at = %s", trigger.NextFireAt)
	}
	if trigger.LastFiredAt.Location() != time.UTC || !trigger.LastFiredAt.Equal(last.UTC()) {
		t.Fatalf("last fired at = %s", trigger.LastFiredAt)
	}
}

func TestNormalizeTriggerRejectsInvalidManagedAndScheduleInput(t *testing.T) {
	if _, err := Normalize(Loader{
		Summary: LoaderSummary{
			ID:               "loader-1",
			ManagedProjectID: "project-1",
		},
		Script: "console.log('ok')",
	}, false); err == nil {
		t.Fatal("expected managed loader validation error")
	}

	if _, err := NormalizeTrigger("loader-1", LoaderTrigger{
		ID:   "cron-1",
		Kind: LoaderTriggerKindCron,
	}); err == nil {
		t.Fatal("expected cron spec validation error")
	}
}
