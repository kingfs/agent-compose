package domain

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"strings"
	"time"
)

const (
	LoaderRuntimeScheduler = "scheduler"

	LoaderTriggerKindInterval = "interval"
	LoaderTriggerKindEvent    = "event"
	LoaderTriggerKindTimeout  = "timeout"
	LoaderTriggerKindCron     = "cron"

	LoaderSessionPolicySticky = "sticky"
	LoaderSessionPolicyNew    = "new"
	LoaderSessionPolicyReuse  = "reuse"

	LoaderConcurrencyPolicySkip     = "skip"
	LoaderConcurrencyPolicyParallel = "parallel"

	LoaderRunStatusRunning   = "running"
	LoaderRunStatusSucceeded = "succeeded"
	LoaderRunStatusFailed    = "failed"
	LoaderRunStatusSkipped   = "skipped"
)

func NormalizeLoaderRuntime(runtime string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(runtime)) {
	case "", LoaderRuntimeScheduler:
		return LoaderRuntimeScheduler, nil
	default:
		return "", fmt.Errorf("unsupported loader runtime %q", runtime)
	}
}

func NormalizeLoaderTriggerKind(kind string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case LoaderTriggerKindInterval:
		return LoaderTriggerKindInterval, nil
	case LoaderTriggerKindEvent:
		return LoaderTriggerKindEvent, nil
	case LoaderTriggerKindTimeout:
		return LoaderTriggerKindTimeout, nil
	case LoaderTriggerKindCron:
		return LoaderTriggerKindCron, nil
	default:
		return "", fmt.Errorf("unsupported loader trigger kind %q", kind)
	}
}

func NormalizeLoaderSessionPolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "", LoaderSessionPolicySticky, LoaderSessionPolicyReuse:
		return LoaderSessionPolicySticky
	case LoaderSessionPolicyNew:
		return LoaderSessionPolicyNew
	default:
		return LoaderSessionPolicySticky
	}
}

func NormalizeLoaderConcurrencyPolicy(policy string) string {
	switch strings.ToLower(strings.TrimSpace(policy)) {
	case "", LoaderConcurrencyPolicySkip:
		return LoaderConcurrencyPolicySkip
	case LoaderConcurrencyPolicyParallel, "allow":
		return LoaderConcurrencyPolicyParallel
	default:
		return LoaderConcurrencyPolicySkip
	}
}

func NormalizeLoaderRunStatus(status string) string {
	switch strings.ToLower(strings.TrimSpace(status)) {
	case LoaderRunStatusRunning:
		return LoaderRunStatusRunning
	case LoaderRunStatusSucceeded:
		return LoaderRunStatusSucceeded
	case LoaderRunStatusFailed:
		return LoaderRunStatusFailed
	case LoaderRunStatusSkipped:
		return LoaderRunStatusSkipped
	default:
		return LoaderRunStatusRunning
	}
}

func LoaderTriggerStableID(kind, topic string, intervalMs int64, callbackSource string, index int) string {
	h := sha256.Sum256([]byte(fmt.Sprintf("%s|%s|%d|%s|%d", kind, topic, intervalMs, callbackSource, index)))
	return "auto-" + hex.EncodeToString(h[:6])
}

func LoaderSourceSHA(script string) string {
	h := sha256.Sum256([]byte(script))
	return hex.EncodeToString(h[:])
}

func LoaderTriggerTopicMatches(pattern, topic string) bool {
	pattern = strings.TrimSpace(pattern)
	topic = strings.TrimSpace(topic)
	if pattern == "" || topic == "" {
		return false
	}
	if pattern == topic {
		return true
	}
	if strings.HasSuffix(pattern, "*") {
		return strings.HasPrefix(topic, strings.TrimSuffix(pattern, "*"))
	}
	return false
}

func LoaderTriggerUsesSchedule(kind string) bool {
	switch strings.ToLower(strings.TrimSpace(kind)) {
	case LoaderTriggerKindInterval, LoaderTriggerKindTimeout, LoaderTriggerKindCron:
		return true
	default:
		return false
	}
}

func LoaderTriggerScheduledAt(now time.Time, delayMs int64) time.Time {
	if delayMs <= 0 {
		return time.Time{}
	}
	return now.UTC().Add(time.Duration(delayMs) * time.Millisecond)
}
