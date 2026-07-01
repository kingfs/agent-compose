package agentcompose

import (
	sessionmodel "agent-compose/pkg/agentcompose/session"
	"context"
)

func newServiceSessionLifecycle(s *Service) *sessionmodel.Lifecycle {
	return sessionmodel.NewLifecycle(
		s.config,
		s.store,
		s.driver,
		s.streams,
		s.configDB,
		sessionmodel.LifecycleHooks{
			PrepareWorkspace: func(ctx context.Context, item *Session) error {
				return prepareSessionWorkspace(ctx, s.config, s.configDB, item)
			},
			WriteCapability: func(ctx context.Context, item *Session, capsetIDs []string) {
				writeCapabilityGuide(ctx, s.cap, s.store, s.streams, item, capsetIDs)
			},
			PublishTopic: func(topic string, item *Session, source string) {
				s.publishLoaderTopic(topic, sessionTopicPayload(item, source))
			},
			NotifyDashboard: func(reason string) {
				if s.dashboard != nil {
					s.dashboard.Notify(reason)
				}
			},
			JupyterReachable: jupyterTargetReachable,
			IsSessionAlive: func(ctx context.Context, driver string, item *Session, vmState VMState) (bool, bool, error) {
				if s.runtimes == nil {
					return false, false, nil
				}
				runtime, err := s.runtimes.ForDriver(driver)
				if err != nil {
					return false, false, err
				}
				aliveRuntime, ok := runtime.(sessionAliveRuntime)
				if !ok {
					return false, false, nil
				}
				alive, err := aliveRuntime.IsSessionAlive(ctx, item, vmState)
				return alive, true, err
			},
		},
	)
}

func serviceReconcileSessionRuntimeState(ctx context.Context, s *Service, item *Session) (*Session, error) {
	if s.sessions != nil {
		return s.sessions.reconcileSessionRuntimeState(ctx, item)
	}
	return newServiceSessionLifecycle(s).ReconcileRuntimeState(ctx, item)
}

func serviceEnsureSessionProxyReady(ctx context.Context, s *Service, sessionID string) (*Session, ProxyState, error) {
	if s.sessions != nil {
		return s.sessions.ensureSessionProxyReady(ctx, sessionID)
	}
	return newServiceSessionLifecycle(s).EnsureProxyReady(ctx, sessionID)
}
