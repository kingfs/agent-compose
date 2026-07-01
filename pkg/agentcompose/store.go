package agentcompose

import (
	"context"

	sessionfs "agent-compose/pkg/agentcompose/store/sessionfs"
	appconfig "agent-compose/pkg/config"

	"github.com/samber/do/v2"
)

type Store struct {
	config *appconfig.Config
	inner  *sessionfs.Store
}

func NewStore(di do.Injector) (*Store, error) {
	inner, err := sessionfs.NewStore(di)
	if err != nil {
		return nil, err
	}
	return &Store{config: inner.Config(), inner: inner}, nil
}

func (s *Store) sessionStore() *sessionfs.Store {
	if s == nil {
		return nil
	}
	if s.inner == nil {
		s.inner = sessionfs.NewStoreWithConfig(s.config)
	}
	return s.inner
}

func cloneSessionWorkspace(item *SessionWorkspace) *SessionWorkspace {
	return sessionfs.CloneSessionWorkspace(item)
}

func (s *Store) CreateSession(ctx context.Context, title, baseWorkspace, driver, guestImage, workspaceID, triggerSource string, workspace *SessionWorkspace, envItems []SessionEnvVar, tags []SessionTag) (*Session, error) {
	return s.sessionStore().CreateSession(ctx, title, baseWorkspace, driver, guestImage, workspaceID, triggerSource, workspace, envItems, tags)
}

func (s *Store) GetSession(ctx context.Context, id string) (*Session, error) {
	return s.sessionStore().GetSession(ctx, id)
}

func (s *Store) ListSessions(ctx context.Context, options SessionListOptions) (SessionListResult, error) {
	return s.sessionStore().ListSessions(ctx, options)
}

func (s *Store) UpdateSession(ctx context.Context, session *Session) error {
	return s.sessionStore().UpdateSession(ctx, session)
}

func (s *Store) AddCell(ctx context.Context, session *Session, cell NotebookCell) error {
	return s.sessionStore().AddCell(ctx, session, cell)
}

func (s *Store) ListCells(ctx context.Context, id string) ([]NotebookCell, error) {
	return s.sessionStore().ListCells(ctx, id)
}

func (s *Store) AddAgentRun(ctx context.Context, sessionID string, run AgentRun) error {
	return s.sessionStore().AddAgentRun(ctx, sessionID, run)
}

func (s *Store) AddEvent(ctx context.Context, sessionID string, event SessionEvent) error {
	return s.sessionStore().AddEvent(ctx, sessionID, event)
}

func (s *Store) ListEvents(ctx context.Context, id string) ([]SessionEvent, error) {
	return s.sessionStore().ListEvents(ctx, id)
}

func (s *Store) sessionDir(id string) string {
	return s.sessionStore().SessionDir(id)
}

func (s *Store) vmStatePath(id string) string {
	return s.sessionStore().VMStatePath(id)
}

func (s *Store) legacyVMStatePath(id string) string {
	return s.sessionStore().LegacyVMStatePath(id)
}

func (s *Store) proxyStatePath(id string) string {
	return s.sessionStore().ProxyStatePath(id)
}

func (s *Store) loadSession(id string) (*Session, error) {
	return s.sessionStore().LoadSession(id)
}

func (s *Store) saveSession(session *Session) error {
	return s.sessionStore().SaveSession(session)
}

func (s *Store) GetVMState(id string) (VMState, error) {
	return s.sessionStore().GetVMState(id)
}

func (s *Store) SaveVMState(id string, state VMState) error {
	return s.sessionStore().SaveVMState(id, state)
}

func (s *Store) GetProxyState(id string) (ProxyState, error) {
	return s.sessionStore().GetProxyState(id)
}

func (s *Store) SaveProxyState(id string, state ProxyState) error {
	return s.sessionStore().SaveProxyState(id, state)
}

func (s *Store) saveCells(id string, cells []NotebookCell) error {
	return s.sessionStore().SaveCells(id, cells)
}

func (s *Store) saveEvents(id string, events []SessionEvent) error {
	return s.sessionStore().SaveEvents(id, events)
}
