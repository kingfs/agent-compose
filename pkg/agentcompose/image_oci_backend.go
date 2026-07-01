package agentcompose

import (
	"agent-compose/pkg/agentcompose/images"

	"agent-compose/pkg/imagecache"
)

type OCIImageBackend = images.OCIBackend

func NewOCIImageBackend(cache *imagecache.Cache, options ...images.OCIBackendOption) *OCIImageBackend {
	return images.NewOCIBackend(cache, options...)
}
