package agentcompose

import (
	"context"

	"connectrpc.com/connect"

	agentcomposeimage "agent-compose/pkg/agentcompose/image"
	"agent-compose/pkg/imagecache"
	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

type ImageBackend = agentcomposeimage.ImageBackend
type ImageListRequest = agentcomposeimage.ImageListRequest
type ImageListResult = agentcomposeimage.ImageListResult
type ImagePullRequest = agentcomposeimage.ImagePullRequest
type ImagePullResult = agentcomposeimage.ImagePullResult
type ImageInspectRequest = agentcomposeimage.ImageInspectRequest
type ImageInspectResult = agentcomposeimage.ImageInspectResult
type ImageRemoveRequest = agentcomposeimage.ImageRemoveRequest
type ImageRemoveResult = agentcomposeimage.ImageRemoveResult
type DockerImageClient = agentcomposeimage.DockerImageClient
type DockerImageBackend = agentcomposeimage.DockerImageBackend
type OCIImageBackend = agentcomposeimage.OCIImageBackend
type AutoImageBackend = agentcomposeimage.AutoImageBackend
type imageBackendOpError = agentcomposeimage.BackendOpError

var NewDockerImageBackend = agentcomposeimage.NewDockerImageBackend
var NewDockerImageBackendWithClient = agentcomposeimage.NewDockerImageBackendWithClient
var NewOCIImageBackend = agentcomposeimage.NewOCIImageBackend
var NewOCIImageBackendWithClock = agentcomposeimage.NewOCIImageBackendWithClock
var NewAutoImageBackend = agentcomposeimage.NewAutoImageBackend
var NewAutoImageBackendWithPing = agentcomposeimage.NewAutoImageBackendWithPing

func (s *Service) ListImages(ctx context.Context, req *connect.Request[agentcomposev2.ListImagesRequest]) (*connect.Response[agentcomposev2.ListImagesResponse], error) {
	return s.imageService().ListImages(ctx, req)
}

func (s *Service) PullImage(ctx context.Context, req *connect.Request[agentcomposev2.PullImageRequest]) (*connect.Response[agentcomposev2.PullImageResponse], error) {
	return s.imageService().PullImage(ctx, req)
}

func (s *Service) InspectImage(ctx context.Context, req *connect.Request[agentcomposev2.InspectImageRequest]) (*connect.Response[agentcomposev2.InspectImageResponse], error) {
	return s.imageService().InspectImage(ctx, req)
}

func (s *Service) RemoveImage(ctx context.Context, req *connect.Request[agentcomposev2.RemoveImageRequest]) (*connect.Response[agentcomposev2.RemoveImageResponse], error) {
	return s.imageService().RemoveImage(ctx, req)
}

func (s *Service) imageService() *agentcomposeimage.Service {
	if s == nil {
		return agentcomposeimage.NewService(nil, nil, nil)
	}
	return agentcomposeimage.NewService(s.images, s.ociImages, s.autoImages)
}

func ociMetadataToProtoImage(image imagecache.ImageMetadata, inspectedAt string) *agentcomposev2.Image {
	return agentcomposeimage.MetadataToProtoImage(image, inspectedAt)
}
