package agentcompose

import (
	"context"
	"time"

	"agent-compose/pkg/agentcompose/event"
)

func (s *ConfigStore) eventStore() *event.Store {
	return s.sqliteStore().EventRepository()
}

func (s *ConfigStore) ensureEventSchema(ctx context.Context) error {
	return s.eventStore().EnsureSchema(ctx)
}

func normalizeTopicEventRecord(item TopicEventRecord, assignID bool) (TopicEventRecord, error) {
	return event.NormalizeTopicEventRecord(item, assignID)
}

func webhookSourceTopicMatches(topic, topicPrefix string) bool {
	return event.WebhookSourceTopicMatches(topic, topicPrefix)
}

func (s *ConfigStore) CreateEvent(ctx context.Context, item TopicEventRecord) (TopicEventRecord, error) {
	return s.eventStore().CreateEvent(ctx, item)
}

func (s *ConfigStore) GetEvent(ctx context.Context, eventID string) (TopicEventRecord, error) {
	return s.eventStore().GetEvent(ctx, eventID)
}

func (s *ConfigStore) FindEventByIdempotencyKey(ctx context.Context, topic, key string) (TopicEventRecord, bool, error) {
	return s.eventStore().FindEventByIdempotencyKey(ctx, topic, key)
}

func (s *ConfigStore) ListPendingEvents(ctx context.Context, limit int) ([]TopicEventRecord, error) {
	return s.eventStore().ListPendingEvents(ctx, limit)
}

func (s *ConfigStore) ListEvents(ctx context.Context, filter TopicEventFilter) ([]TopicEventRecord, error) {
	return s.eventStore().ListEvents(ctx, filter)
}

func (s *ConfigStore) MarkEventPublished(ctx context.Context, eventID, claimID string, dispatchedAt time.Time) error {
	return s.eventStore().MarkEventPublished(ctx, eventID, claimID, dispatchedAt)
}

func (s *ConfigStore) UpdateEventPayload(ctx context.Context, eventID, payloadJSON string) error {
	return s.eventStore().UpdateEventPayload(ctx, eventID, payloadJSON)
}

func (s *ConfigStore) ListDispatchableEvents(ctx context.Context, now time.Time, limit int) ([]TopicEventRecord, error) {
	return s.eventStore().ListDispatchableEvents(ctx, now, limit)
}

func (s *ConfigStore) ClaimEvent(ctx context.Context, eventID, claimID string, now, until time.Time) (bool, error) {
	return s.eventStore().ClaimEvent(ctx, eventID, claimID, now, until)
}

func (s *ConfigStore) ReleaseEventClaim(ctx context.Context, eventID, claimID, status, lastError string, nextAttemptAt time.Time) error {
	return s.eventStore().ReleaseEventClaim(ctx, eventID, claimID, status, lastError, nextAttemptAt)
}

func (s *ConfigStore) MarkEventNoSubscriber(ctx context.Context, eventID, claimID string, dispatchedAt time.Time) error {
	return s.eventStore().MarkEventNoSubscriber(ctx, eventID, claimID, dispatchedAt)
}

func (s *ConfigStore) UpsertEventDelivery(ctx context.Context, delivery EventDelivery) error {
	return s.eventStore().UpsertEventDelivery(ctx, delivery)
}

func (s *ConfigStore) AddEventSessionLink(ctx context.Context, link EventSessionLink) error {
	return s.eventStore().AddEventSessionLink(ctx, link)
}

func (s *ConfigStore) ListEventDeliveries(ctx context.Context, eventIDs []string) ([]EventDelivery, error) {
	return s.eventStore().ListEventDeliveries(ctx, eventIDs)
}

func (s *ConfigStore) ListEventSessionLinks(ctx context.Context, eventIDs []string) ([]EventSessionTraceItem, error) {
	return s.eventStore().ListEventSessionLinks(ctx, eventIDs)
}

func (s *ConfigStore) ListDescendantEventIDs(ctx context.Context, rootEventID string, limit int) ([]string, error) {
	return s.eventStore().ListDescendantEventIDs(ctx, rootEventID, limit)
}

func (s *ConfigStore) ListEnabledWebhookSourcesForTopic(ctx context.Context, topic string) ([]WebhookSource, error) {
	return s.eventStore().ListEnabledWebhookSourcesForTopic(ctx, topic)
}

func (s *ConfigStore) ListWebhookSources(ctx context.Context) ([]WebhookSource, error) {
	return s.eventStore().ListWebhookSources(ctx)
}

func (s *ConfigStore) GetWebhookSource(ctx context.Context, sourceID string) (WebhookSource, bool, error) {
	return s.eventStore().GetWebhookSource(ctx, sourceID)
}

func (s *ConfigStore) UpsertWebhookSource(ctx context.Context, source WebhookSource) (WebhookSource, error) {
	return s.eventStore().UpsertWebhookSource(ctx, source)
}

func (s *ConfigStore) DeleteWebhookSource(ctx context.Context, sourceID string) error {
	return s.eventStore().DeleteWebhookSource(ctx, sourceID)
}
