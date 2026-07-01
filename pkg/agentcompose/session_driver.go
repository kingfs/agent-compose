package agentcompose

import (
	sessionruntime "agent-compose/pkg/agentcompose/runtime"
	appconfig "agent-compose/pkg/config"
	"context"

	"github.com/samber/do/v2"
)

type Driver = sessionruntime.Driver

type SessionDriver struct {
	config   *appconfig.Config
	store    *Store
	configDB *ConfigStore
	runtimes RuntimeProvider
	inner    *sessionruntime.SessionDriver
}

func NewDriver(di do.Injector) (Driver, error) {
	return &SessionDriver{
		config:   do.MustInvoke[*appconfig.Config](di),
		store:    do.MustInvoke[*Store](di),
		configDB: do.MustInvoke[*ConfigStore](di),
		runtimes: do.MustInvoke[RuntimeProvider](di),
	}, nil
}

func (d *SessionDriver) sessionDriver() *sessionruntime.SessionDriver {
	if d.inner == nil {
		d.inner = sessionruntime.NewSessionDriver(d.config, d.store, d.configDB, d.runtimes, d.prepareRuntimeEnv)
	}
	return d.inner
}

func (d *SessionDriver) StartSessionVM(ctx context.Context, session *Session) error {
	return d.sessionDriver().StartSessionVM(ctx, session)
}

func (d *SessionDriver) StopSessionVM(ctx context.Context, session *Session) error {
	return d.sessionDriver().StopSessionVM(ctx, session)
}

func (d *SessionDriver) saveSessionStartInfo(session *Session, vmState VMState, proxyState ProxyState, info SessionVMInfo) error {
	return d.sessionDriver().SaveSessionStartInfo(session, vmState, proxyState, info)
}

func (d *SessionDriver) prepareRuntimeEnv(ctx context.Context, session *Session) ([]SessionEnvVar, error) {
	managedEnv, err := ensureSessionLLMFacadeConfig(ctx, d.config, d.configDB, session, "codex", "", "session", "")
	if err != nil {
		return nil, err
	}
	return envItemsFromMap(managedEnv, false), nil
}
