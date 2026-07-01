package agentcompose

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"strings"

	webhookhandler "agent-compose/pkg/agentcompose/webhook"

	"github.com/labstack/echo/v4"
)

type webhookAcceptedResponse struct {
	Accepted      bool   `json:"accepted"`
	Topic         string `json:"topic"`
	EventID       string `json:"event_id"`
	Sequence      int64  `json:"sequence"`
	CorrelationID string `json:"correlation_id"`
}

type topicEventListResponse struct {
	Items             []topicEventJSON `json:"items"`
	NextAfterSequence int64            `json:"next_after_sequence"`
}

type eventSessionsResponse struct {
	EventID       string             `json:"event_id"`
	CorrelationID string             `json:"correlation_id"`
	Sessions      []eventSessionJSON `json:"sessions"`
}

type eventRunsResponse struct {
	EventID       string         `json:"event_id"`
	CorrelationID string         `json:"correlation_id"`
	Runs          []eventRunJSON `json:"runs"`
}

type eventRunJSON struct {
	EventID   string `json:"event_id"`
	LoaderID  string `json:"loader_id"`
	RunID     string `json:"run_id,omitempty"`
	TriggerID string `json:"trigger_id"`
	Status    string `json:"status"`
	Error     string `json:"error,omitempty"`
	CreatedAt string `json:"created_at"`
	UpdatedAt string `json:"updated_at"`
}

type eventSessionJSON struct {
	SessionID     string `json:"session_id"`
	Relation      string `json:"relation"`
	LoaderID      string `json:"loader_id,omitempty"`
	RunID         string `json:"run_id,omitempty"`
	TriggerID     string `json:"trigger_id,omitempty"`
	LoaderEventID string `json:"loader_event_id,omitempty"`
	EventID       string `json:"event_id"`
	CreatedAt     string `json:"created_at"`
}

type webhookSourceJSON struct {
	ID                 string `json:"id"`
	Name               string `json:"name"`
	Enabled            bool   `json:"enabled"`
	Provider           string `json:"provider"`
	TopicPrefix        string `json:"topic_prefix"`
	HasToken           bool   `json:"has_token"`
	SignatureType      string `json:"signature_type,omitempty"`
	HasSignatureSecret bool   `json:"has_signature_secret"`
	BodyLimitBytes     int64  `json:"body_limit_bytes,omitempty"`
	CreatedAt          string `json:"created_at"`
	UpdatedAt          string `json:"updated_at"`
}

type webhookSourceListResponse struct {
	Items []webhookSourceJSON `json:"items"`
}

type webhookSourceResponse struct {
	Source webhookSourceJSON `json:"source"`
}

type topicEventJSON struct {
	EventID        string         `json:"event_id"`
	Sequence       int64          `json:"sequence"`
	Topic          string         `json:"topic"`
	Source         string         `json:"source"`
	Provider       string         `json:"provider,omitempty"`
	Intent         string         `json:"intent,omitempty"`
	CorrelationID  string         `json:"correlation_id"`
	IdempotencyKey string         `json:"idempotency_key,omitempty"`
	DeliveryID     string         `json:"delivery_id,omitempty"`
	DispatchStatus string         `json:"dispatch_status"`
	ParentEventID  string         `json:"parent_event_id,omitempty"`
	PublisherType  string         `json:"publisher_type,omitempty"`
	PublisherID    string         `json:"publisher_id,omitempty"`
	PublisherRunID string         `json:"publisher_run_id,omitempty"`
	CreatedAt      string         `json:"created_at"`
	DispatchedAt   string         `json:"dispatched_at,omitempty"`
	Payload        map[string]any `json:"payload"`
}

func registerWebhookRoutes(app *echo.Echo, service *Service) {
	webhookhandler.RegisterRoutes(app, webhookhandler.NewHandler(service.config.WebhookBodyLimitBytes, service.configDB))
}

func webhookTokenHash(token string) string {
	sum := sha256.Sum256([]byte(strings.TrimSpace(token)))
	return "sha256:" + hex.EncodeToString(sum[:])
}

func providerFromWebhookTopic(topic string) string {
	parts := strings.Split(topic, ".")
	if len(parts) >= 2 && parts[0] == "webhook" {
		return strings.TrimSpace(parts[1])
	}
	return ""
}

func existingWebhookBodyHash(payloadJSON string) string {
	var payload map[string]any
	if err := json.Unmarshal([]byte(payloadJSON), &payload); err != nil {
		return ""
	}
	body, ok := payload["body"]
	if !ok {
		return ""
	}
	compact, err := marshalJSONCompact(body)
	if err != nil {
		return ""
	}
	return topicEventPayloadSHA256(compact)
}
