package agentcompose

import (
	"time"

	"agent-compose/pkg/agentcompose/loader"
)

const (
	LoaderRuntimeScheduler = loader.LoaderRuntimeScheduler

	LoaderTriggerKindInterval = loader.LoaderTriggerKindInterval
	LoaderTriggerKindEvent    = loader.LoaderTriggerKindEvent
	LoaderTriggerKindTimeout  = loader.LoaderTriggerKindTimeout
	LoaderTriggerKindCron     = loader.LoaderTriggerKindCron

	LoaderSessionPolicySticky = loader.LoaderSessionPolicySticky
	LoaderSessionPolicyNew    = loader.LoaderSessionPolicyNew
	LoaderSessionPolicyReuse  = loader.LoaderSessionPolicyReuse

	LoaderConcurrencyPolicySkip     = loader.LoaderConcurrencyPolicySkip
	LoaderConcurrencyPolicyParallel = loader.LoaderConcurrencyPolicyParallel

	LoaderRunStatusRunning   = loader.LoaderRunStatusRunning
	LoaderRunStatusSucceeded = loader.LoaderRunStatusSucceeded
	LoaderRunStatusFailed    = loader.LoaderRunStatusFailed
	LoaderRunStatusSkipped   = loader.LoaderRunStatusSkipped
)

type LoaderSummary = loader.LoaderSummary
type Loader = loader.Loader
type LoaderTrigger = loader.LoaderTrigger
type LoaderRunSummary = loader.LoaderRunSummary
type LoaderEvent = loader.LoaderEvent
type LoaderBinding = loader.LoaderBinding
type LoaderAgentRequest = loader.LoaderAgentRequest
type LoaderAgentResult = loader.LoaderAgentResult
type LoaderCommandRequest = loader.LoaderCommandRequest
type LoaderCommandResult = loader.LoaderCommandResult
type LoaderLLMRequest = loader.LoaderLLMRequest
type LoaderLLMResult = loader.LoaderLLMResult
type LoaderTopicEvent = loader.LoaderTopicEvent

func normalizeLoaderRuntime(runtime string) (string, error) {
	return loader.NormalizeRuntime(runtime)
}

func normalizeLoaderTriggerKind(kind string) (string, error) {
	return loader.NormalizeTriggerKind(kind)
}

func normalizeLoaderSessionPolicy(policy string) string {
	return loader.NormalizeSessionPolicy(policy)
}

func normalizeLoaderConcurrencyPolicy(policy string) string {
	return loader.NormalizeConcurrencyPolicy(policy)
}

func normalizeLoaderRunStatus(status string) string {
	return loader.NormalizeRunStatus(status)
}

func loaderTriggerStableID(kind, topic string, intervalMs int64, callbackSource string, index int) string {
	return loader.TriggerStableID(kind, topic, intervalMs, callbackSource, index)
}

func loaderSourceSHA(script string) string {
	return loader.SourceSHA(script)
}

func loaderTriggerTopicMatches(pattern, topic string) bool {
	return loader.TriggerTopicMatches(pattern, topic)
}

func timeIsSet(value time.Time) bool {
	return loader.TimeIsSet(value)
}

func nonZeroTimeUnixMilli(value time.Time) int64 {
	return loader.NonZeroTimeUnixMilli(value)
}

func loaderTriggerUsesSchedule(kind string) bool {
	return loader.TriggerUsesSchedule(kind)
}

func loaderTriggerScheduledAt(now time.Time, delayMs int64) time.Time {
	return loader.TriggerScheduledAt(now, delayMs)
}

func defaultLoaderName(now time.Time) string {
	return loader.DefaultName(now)
}

func defaultLoaderScript() string {
	return loader.DefaultScript()
}
