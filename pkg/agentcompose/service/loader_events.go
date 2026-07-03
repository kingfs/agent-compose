package agentcompose

import (
	"time"

	domain "agent-compose/pkg/model"
)

func (s *Service) publishLoaderTopic(topic string, payload map[string]any) {
	if s == nil || s.bus == nil {
		return
	}
	s.bus.Publish(domain.LoaderTopicEvent{
		Topic:     topic,
		Payload:   payload,
		CreatedAt: time.Now().UTC(),
	})
}
