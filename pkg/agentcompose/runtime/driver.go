package runtime

import (
	"context"
	"time"

	"agent-compose/pkg/agentcompose/session"
	appconfig "agent-compose/pkg/config"
	driverpkg "agent-compose/pkg/driver"
)

type Store interface {
	GetVMState(string) (session.VMState, error)
	SaveVMState(string, session.VMState) error
	GetProxyState(string) (session.ProxyState, error)
	SaveProxyState(string, session.ProxyState) error
}

type TokenRevoker interface {
	RevokeLLMFacadeTokensForSession(context.Context, string) error
}

type PrepareRuntimeEnvFunc func(context.Context, *session.Session) ([]session.SessionEnvVar, error)

type SessionDriver struct {
	config            *appconfig.Config
	store             Store
	configDB          TokenRevoker
	runtimes          RuntimeProvider
	prepareRuntimeEnv PrepareRuntimeEnvFunc
}

func NewSessionDriver(config *appconfig.Config, store Store, configDB TokenRevoker, runtimes RuntimeProvider, prepareRuntimeEnv PrepareRuntimeEnvFunc) *SessionDriver {
	return &SessionDriver{
		config:            config,
		store:             store,
		configDB:          configDB,
		runtimes:          runtimes,
		prepareRuntimeEnv: prepareRuntimeEnv,
	}
}

func (d *SessionDriver) StartSessionVM(ctx context.Context, item *session.Session) error {
	ctx, cancel := context.WithTimeout(ctx, d.config.SessionStartTimeout)
	defer cancel()

	driver, err := driverpkg.ResolveSessionRuntimeDriver(item.Summary.Driver, d.config.RuntimeDriver)
	if err != nil {
		return err
	}
	runtime, err := d.runtimes.ForDriver(driver)
	if err != nil {
		return err
	}

	vmState, err := d.store.GetVMState(item.Summary.ID)
	if err != nil {
		return err
	}
	proxyState, err := d.store.GetProxyState(item.Summary.ID)
	if err != nil {
		return err
	}
	vmState.Driver = driver
	vmState.Mode = driver
	vmState.BoxName = firstNonEmpty(vmState.BoxName, item.Summary.RuntimeRef)
	vmState.RuntimeHome = firstNonEmpty(vmState.RuntimeHome, driverpkg.RuntimeHomeForDriver(d.config, driver))
	if err := d.prepareSessionStart(ctx, driver, item, &vmState); err != nil {
		vmState.LastError = err.Error()
		_ = d.store.SaveVMState(item.Summary.ID, vmState)
		return err
	}

	info, err := runtime.EnsureSession(ctx, item, vmState, proxyState)
	if err != nil {
		vmState.LastError = err.Error()
		vmState.StoppedAt = time.Time{}
		_ = d.store.SaveVMState(item.Summary.ID, vmState)
		return err
	}

	return d.SaveSessionStartInfo(item, vmState, proxyState, info)
}

func (d *SessionDriver) SaveSessionStartInfo(item *session.Session, vmState session.VMState, proxyState session.ProxyState, info SessionVMInfo) error {
	vmState.BoxID = firstNonEmpty(info.BoxID, vmState.BoxID)
	vmState.StartedAt = time.Now().UTC()
	vmState.StoppedAt = time.Time{}
	vmState.LastError = ""
	vmState.BootstrapRef = firstNonEmpty(info.JupyterURL, vmState.BootstrapRef)
	if err := d.store.SaveVMState(item.Summary.ID, vmState); err != nil {
		return err
	}
	if info.ProxyState != nil {
		proxyState = *info.ProxyState
	}
	proxyState.JupyterURL = firstNonEmpty(info.JupyterURL, proxyState.JupyterURL)
	return d.store.SaveProxyState(item.Summary.ID, proxyState)
}

func (d *SessionDriver) StopSessionVM(ctx context.Context, item *session.Session) error {
	ctx, cancel := context.WithTimeout(ctx, d.config.SessionStopTimeout)
	defer cancel()

	driver, err := driverpkg.ResolveSessionRuntimeDriver(item.Summary.Driver, d.config.RuntimeDriver)
	if err != nil {
		return err
	}
	runtime, err := d.runtimes.ForDriver(driver)
	if err != nil {
		return err
	}

	vmState, err := d.store.GetVMState(item.Summary.ID)
	if err != nil {
		return err
	}
	missing, err := runtime.StopSession(ctx, item, vmState)
	if err != nil {
		vmState.LastError = err.Error()
		_ = d.store.SaveVMState(item.Summary.ID, vmState)
		return err
	}

	vmState.StoppedAt = time.Now().UTC()
	vmState.LastError = ""
	if missing {
		vmState.BoxID = ""
	}
	if d.configDB != nil {
		if err := d.configDB.RevokeLLMFacadeTokensForSession(ctx, item.Summary.ID); err != nil {
			return err
		}
	}
	return d.store.SaveVMState(item.Summary.ID, vmState)
}

func (d *SessionDriver) prepareSessionStart(ctx context.Context, driver string, item *session.Session, vmState *session.VMState) error {
	prepared, err := driverpkg.PrepareSessionStart(ctx, d.config, driver, toDriverSession(item), toDriverVMState(*vmState))
	if err != nil {
		return err
	}
	if d.prepareRuntimeEnv != nil {
		managedEnv, err := d.prepareRuntimeEnv(ctx, item)
		if err != nil {
			return err
		}
		if len(managedEnv) > 0 {
			item.RuntimeEnvItems = managedEnv
		}
	}
	*vmState = fromDriverVMState(prepared)
	return nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
