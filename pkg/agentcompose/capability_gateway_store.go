package agentcompose

import (
	"context"

	sqlitestore "agent-compose/pkg/agentcompose/store/sqlite"
)

type CapabilityGatewaySettings = sqlitestore.CapabilityGatewaySettings

func (s *ConfigStore) ensureCapabilityGatewaySchema(ctx context.Context) error {
	return s.sqliteStore().EnsureCapabilityGatewaySchema(ctx)
}

// GetCapabilityGateway returns the stored OctoBus connection. An empty addr
// means the gateway is not configured.
func (s *ConfigStore) GetCapabilityGateway(ctx context.Context) (CapabilityGatewaySettings, error) {
	return s.sqliteStore().GetCapabilityGateway(ctx)
}

// SaveCapabilityGateway upserts the OctoBus connection settings.
func (s *ConfigStore) SaveCapabilityGateway(ctx context.Context, settings CapabilityGatewaySettings) (CapabilityGatewaySettings, error) {
	return s.sqliteStore().SaveCapabilityGateway(ctx, settings)
}
