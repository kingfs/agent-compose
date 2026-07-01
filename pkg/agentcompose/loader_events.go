package agentcompose

import (
	"time"

	"agent-compose/pkg/agentcompose/loaders"
)

func (s *Service) publishLoaderTopic(topic string, payload map[string]any) {
	if s == nil || s.bus == nil {
		return
	}
	s.bus.Publish(LoaderTopicEvent{
		Topic:     topic,
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	})
}

func sessionTopicPayload(session *Session, source string) map[string]any {
	return loaders.SessionTopicPayload(session, source)
}

func cellTopicPayload(sessionID string, cell NotebookCell, source string) map[string]any {
	return loaders.CellTopicPayload(sessionID, cell, source)
}

func loaderCommandEventPayload(request LoaderCommandRequest, result LoaderCommandResult) map[string]any {
	return loaders.CommandEventPayload(request, result)
}
