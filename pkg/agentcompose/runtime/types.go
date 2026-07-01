package runtime

import (
	"context"

	"agent-compose/pkg/agentcompose/session"
)

type SessionVMInfo struct {
	BoxID      string
	JupyterURL string
	ProxyState *session.ProxyState
}

type Driver interface {
	StartSessionVM(context.Context, *session.Session) error
	StopSessionVM(context.Context, *session.Session) error
}

type BoxRuntime interface {
	EnsureSession(context.Context, *session.Session, session.VMState, session.ProxyState) (SessionVMInfo, error)
	StopSession(context.Context, *session.Session, session.VMState) (bool, error)
	Exec(context.Context, *session.Session, session.VMState, session.ExecSpec) (session.ExecResult, error)
	ExecStream(context.Context, *session.Session, session.VMState, session.ExecSpec, session.ExecStreamWriter) (session.ExecResult, error)
}

type RuntimeProvider interface {
	ForDriver(string) (BoxRuntime, error)
	ForSession(*session.Session) (BoxRuntime, error)
}
