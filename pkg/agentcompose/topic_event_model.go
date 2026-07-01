package agentcompose

import "agent-compose/pkg/agentcompose/event"

const (
	TopicEventSourceWebhook = event.TopicEventSourceWebhook
	TopicEventSourceLoader  = event.TopicEventSourceLoader
	TopicEventSourceSystem  = event.TopicEventSourceSystem

	TopicEventDispatchPending        = event.TopicEventDispatchPending
	TopicEventDispatchPublishing     = event.TopicEventDispatchPublishing
	TopicEventDispatchPublishedToBus = event.TopicEventDispatchPublishedToBus
	TopicEventDispatchNoSubscriber   = event.TopicEventDispatchNoSubscriber
	TopicEventDispatchRetrying       = event.TopicEventDispatchRetrying
	TopicEventDispatchDeadLetter     = event.TopicEventDispatchDeadLetter

	EventDeliveryStatusMatched      = event.EventDeliveryStatusMatched
	EventDeliveryStatusRunStarted   = event.EventDeliveryStatusRunStarted
	EventDeliveryStatusRunSucceeded = event.EventDeliveryStatusRunSucceeded
	EventDeliveryStatusRunFailed    = event.EventDeliveryStatusRunFailed
	EventDeliveryStatusSkipped      = event.EventDeliveryStatusSkipped
)

type TopicEventRecord = event.TopicEventRecord
type TopicEventFilter = event.TopicEventFilter
type WebhookSource = event.WebhookSource
type EventDelivery = event.EventDelivery
type EventSessionLink = event.EventSessionLink
type EventSessionTraceItem = event.EventSessionTraceItem

func validateTopicEventName(topic string) error {
	return event.ValidateTopicEventName(topic)
}

func normalizeTopicEventSource(source string) string {
	return event.NormalizeTopicEventSource(source)
}

func normalizeTopicEventDispatchStatus(status string) string {
	return event.NormalizeTopicEventDispatchStatus(status)
}

func normalizeEventDeliveryStatus(status string) string {
	return event.NormalizeEventDeliveryStatus(status)
}

func topicEventPayloadSHA256(payloadJSON string) string {
	return event.PayloadSHA256(payloadJSON)
}
