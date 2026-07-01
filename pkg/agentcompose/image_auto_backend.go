package agentcompose

import "agent-compose/pkg/agentcompose/images"

type (
	DockerPingFunc   = images.DockerPingFunc
	AutoImageBackend = images.AutoBackend
)

func NewAutoImageBackend(mode string, dockerBackend, ociBackend ImageBackend, options ...images.AutoBackendOption) *AutoImageBackend {
	return images.NewAutoBackend(mode, dockerBackend, ociBackend, options...)
}
