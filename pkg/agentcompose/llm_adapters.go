package agentcompose

import (
	llmpkg "agent-compose/pkg/agentcompose/llm"
	"context"
)

func (s *ConfigStore) llmStore() *llmpkg.Store {
	if s == nil {
		return nil
	}
	return s.sqliteStore().LLMRepository(func(ctx context.Context) ([]llmpkg.GlobalEnvVar, error) {
		items, err := s.ListGlobalEnv(ctx)
		if err != nil {
			return nil, err
		}
		result := make([]llmpkg.GlobalEnvVar, 0, len(items))
		for _, item := range items {
			result = append(result, llmpkg.GlobalEnvVar{Name: item.Name, Value: item.Value})
		}
		return result, nil
	})
}
