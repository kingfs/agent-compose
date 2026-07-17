package cleanup

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"sync"
	"testing"
	"time"
)

func TestRunnerRunOnceUsesIndependentPolicyCutoffs(t *testing.T) {
	now := time.Date(2026, 7, 17, 12, 0, 0, 0, time.UTC)
	first := &recordingCleaner{name: "workspace"}
	second := &recordingCleaner{name: "image", err: errors.New("failed")}
	runner := &Runner{
		Interval: time.Hour,
		Now:      func() time.Time { return now },
		Logger:   slog.New(slog.NewTextHandler(io.Discard, nil)),
		Policies: []Policy{{TTL: 24 * time.Hour, Cleaner: first}, {TTL: 48 * time.Hour, Cleaner: second}, {TTL: 0, Cleaner: &recordingCleaner{name: "disabled"}}},
	}
	runner.runOnce(context.Background())
	if len(first.cutoffs) != 1 || !first.cutoffs[0].Equal(now.Add(-24*time.Hour)) {
		t.Fatalf("workspace cutoffs = %v", first.cutoffs)
	}
	if len(second.cutoffs) != 1 || !second.cutoffs[0].Equal(now.Add(-48*time.Hour)) {
		t.Fatalf("image cutoffs = %v", second.cutoffs)
	}
}

func TestRunnerDisabledWithoutPositivePolicy(t *testing.T) {
	if (&Runner{Interval: time.Hour, Policies: []Policy{{TTL: 0, Cleaner: &recordingCleaner{}}}}).Enabled() {
		t.Fatal("runner with zero TTL is enabled")
	}
	if (&Runner{Interval: 0, Policies: []Policy{{TTL: time.Hour, Cleaner: &recordingCleaner{}}}}).Enabled() {
		t.Fatal("runner with zero interval is enabled")
	}
}

func TestRunnerShutdownWaitsForActiveCleaner(t *testing.T) {
	cleaner := &blockingCleaner{entered: make(chan struct{}), release: make(chan struct{})}
	runner := &Runner{Interval: time.Hour, Policies: []Policy{{TTL: time.Hour, Cleaner: cleaner}}}
	runner.Start(context.Background())
	<-cleaner.entered

	shutdownDone := make(chan error, 1)
	go func() { shutdownDone <- runner.Shutdown(context.Background()) }()
	select {
	case err := <-shutdownDone:
		t.Fatalf("Shutdown returned before cleaner completed: %v", err)
	case <-time.After(20 * time.Millisecond):
	}
	close(cleaner.release)
	if err := <-shutdownDone; err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}
}

type recordingCleaner struct {
	name    string
	err     error
	cutoffs []time.Time
}

type blockingCleaner struct {
	entered chan struct{}
	release chan struct{}
	once    sync.Once
}

func (*blockingCleaner) Name() string { return "blocking" }
func (c *blockingCleaner) Clean(context.Context, time.Time) (Result, error) {
	c.once.Do(func() { close(c.entered) })
	<-c.release
	return Result{}, nil
}

func (c *recordingCleaner) Name() string { return c.name }
func (c *recordingCleaner) Clean(_ context.Context, cutoff time.Time) (Result, error) {
	c.cutoffs = append(c.cutoffs, cutoff)
	return Result{Matched: 1}, c.err
}
