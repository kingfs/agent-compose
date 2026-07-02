package image

import (
	"context"
	"errors"
	"testing"
	"time"

	appconfig "agent-compose/pkg/config"
	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

func TestAutoImageBackendUsesDockerWhenAutoPingSucceeds(t *testing.T) {
	dockerCalled := false
	ociCalled := false
	backend := &AutoImageBackend{
		mode: appconfig.ImageStoreModeAuto,
		docker: &fakeImageBackend{listImages: func(ctx context.Context, req ImageListRequest) (ImageListResult, error) {
			dockerCalled = true
			return ImageListResult{StoreStatus: &agentcomposev2.ImageStoreStatus{Store: agentcomposev2.ImageStoreKind_IMAGE_STORE_KIND_DOCKER_DAEMON}}, nil
		}},
		oci: &fakeImageBackend{listImages: func(ctx context.Context, req ImageListRequest) (ImageListResult, error) {
			ociCalled = true
			return ImageListResult{}, nil
		}},
		pingDocker:  func(ctx context.Context) error { return nil },
		pingTimeout: time.Second,
	}

	result, err := backend.ListImages(context.Background(), ImageListRequest{})
	if err != nil {
		t.Fatalf("ListImages returned error: %v", err)
	}
	if !dockerCalled || ociCalled || backend.lastSelection != appconfig.ImageStoreModeDocker || result.StoreStatus.GetStore() != agentcomposev2.ImageStoreKind_IMAGE_STORE_KIND_DOCKER_DAEMON {
		t.Fatalf("selection docker=%v oci=%v last=%q result=%#v", dockerCalled, ociCalled, backend.lastSelection, result)
	}
}

func TestAutoImageBackendUsesOCIWhenAutoPingFails(t *testing.T) {
	dockerCalled := false
	ociCalled := false
	backend := &AutoImageBackend{
		mode: appconfig.ImageStoreModeAuto,
		docker: &fakeImageBackend{pullImage: func(ctx context.Context, req ImagePullRequest) (ImagePullResult, error) {
			dockerCalled = true
			return ImagePullResult{}, nil
		}},
		oci: &fakeImageBackend{pullImage: func(ctx context.Context, req ImagePullRequest) (ImagePullResult, error) {
			ociCalled = true
			return ImagePullResult{ResolvedRef: "oci"}, nil
		}},
		pingDocker:  func(ctx context.Context) error { return errors.New("docker unavailable") },
		pingTimeout: time.Second,
	}

	result, err := backend.PullImage(context.Background(), ImagePullRequest{ImageRef: "team/app:latest"})
	if err != nil {
		t.Fatalf("PullImage returned error: %v", err)
	}
	if dockerCalled || !ociCalled || backend.lastSelection != appconfig.ImageStoreModeOCI || result.ResolvedRef != "oci" {
		t.Fatalf("selection docker=%v oci=%v last=%q result=%#v", dockerCalled, ociCalled, backend.lastSelection, result)
	}
}

func TestAutoImageBackendForcedModesDoNotPing(t *testing.T) {
	for _, tc := range []struct {
		name string
		mode string
		run  func(*AutoImageBackend) error
		want string
	}{
		{
			name: appconfig.ImageStoreModeDocker,
			mode: appconfig.ImageStoreModeDocker,
			run: func(backend *AutoImageBackend) error {
				_, err := backend.InspectImage(context.Background(), ImageInspectRequest{ImageRef: "team/app:latest"})
				return err
			},
			want: appconfig.ImageStoreModeDocker,
		},
		{
			name: appconfig.ImageStoreModeOCI,
			mode: appconfig.ImageStoreModeOCI,
			run: func(backend *AutoImageBackend) error {
				_, err := backend.RemoveImage(context.Background(), ImageRemoveRequest{ImageRef: "team/app:latest"})
				return err
			},
			want: appconfig.ImageStoreModeOCI,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			pinged := false
			dockerCalled := false
			ociCalled := false
			backend := &AutoImageBackend{
				mode: tc.mode,
				docker: &fakeImageBackend{
					inspectImage: func(ctx context.Context, req ImageInspectRequest) (ImageInspectResult, error) {
						dockerCalled = true
						return ImageInspectResult{}, nil
					},
				},
				oci: &fakeImageBackend{
					removeImage: func(ctx context.Context, req ImageRemoveRequest) (ImageRemoveResult, error) {
						ociCalled = true
						return ImageRemoveResult{}, nil
					},
				},
				pingDocker: func(ctx context.Context) error {
					pinged = true
					return nil
				},
			}
			if err := tc.run(backend); err != nil {
				t.Fatalf("operation returned error: %v", err)
			}
			if pinged || backend.lastSelection != tc.want {
				t.Fatalf("pinged=%v last=%q want=%q", pinged, backend.lastSelection, tc.want)
			}
			if tc.want == appconfig.ImageStoreModeDocker && !dockerCalled {
				t.Fatalf("docker backend was not called")
			}
			if tc.want == appconfig.ImageStoreModeOCI && !ociCalled {
				t.Fatalf("oci backend was not called")
			}
		})
	}
}
