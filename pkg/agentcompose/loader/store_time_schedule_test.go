package loader

import (
	"testing"
	"time"
)

func TestParseStoredLoaderTriggerTimeAcceptsSecondsMillisecondsAndRFC3339(t *testing.T) {
	want := time.Date(2026, 7, 2, 12, 34, 56, 789_000_000, time.UTC)

	cases := []struct {
		name  string
		value any
		want  time.Time
	}{
		{name: "nil", value: nil, want: time.Time{}},
		{name: "seconds", value: int64(want.Unix()), want: time.Unix(want.Unix(), 0).UTC()},
		{name: "milliseconds", value: want.UnixMilli(), want: time.UnixMilli(want.UnixMilli()).UTC()},
		{name: "numeric string", value: " 1782995696789 ", want: time.UnixMilli(1782995696789).UTC()},
		{name: "rfc3339 nano bytes", value: []byte(want.Format(time.RFC3339Nano)), want: want},
		{name: "invalid", value: "not a time", want: time.Time{}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := parseStoredLoaderTriggerTime(tc.value)
			if !got.Equal(tc.want) {
				t.Fatalf("parseStoredLoaderTriggerTime(%#v) = %s, want %s", tc.value, got, tc.want)
			}
		})
	}
}

func TestCronSpecJSONNormalizesTimezoneAndNextFire(t *testing.T) {
	specJSON, err := CronSpecJSON("0 9 * * *", "Asia/Shanghai")
	if err != nil {
		t.Fatalf("CronSpecJSON returned error: %v", err)
	}
	if specJSON != `{"kind":"cron","expr":"0 9 * * *","timezone":"Asia/Shanghai"}` {
		t.Fatalf("spec json = %s", specJSON)
	}

	now := time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)
	next, err := TriggerNextFireAt(now, LoaderTrigger{Kind: LoaderTriggerKindCron, SpecJSON: specJSON}, false)
	if err != nil {
		t.Fatalf("TriggerNextFireAt returned error: %v", err)
	}
	want := time.Date(2026, 7, 2, 1, 0, 0, 0, time.UTC)
	if !next.Equal(want) {
		t.Fatalf("next fire = %s, want %s", next, want)
	}
	if source := TriggerSource(LoaderTrigger{Kind: LoaderTriggerKindCron, SpecJSON: specJSON}); source != "cron:0 9 * * *@Asia/Shanghai" {
		t.Fatalf("source = %q", source)
	}
}

func TestTriggerNextFireAtHandlesIntervalAndTimeout(t *testing.T) {
	now := time.Date(2026, 7, 2, 12, 0, 0, 0, time.UTC)

	next, err := TriggerNextFireAt(now, LoaderTrigger{Kind: LoaderTriggerKindInterval, IntervalMs: 2500}, false)
	if err != nil {
		t.Fatalf("interval returned error: %v", err)
	}
	if want := now.Add(2500 * time.Millisecond); !next.Equal(want) {
		t.Fatalf("interval next = %s, want %s", next, want)
	}

	next, err = TriggerNextFireAt(now, LoaderTrigger{Kind: LoaderTriggerKindTimeout, IntervalMs: 2500}, true)
	if err != nil {
		t.Fatalf("timeout returned error: %v", err)
	}
	if !next.IsZero() {
		t.Fatalf("fired timeout next = %s", next)
	}
}
