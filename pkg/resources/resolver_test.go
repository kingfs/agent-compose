package resources

import (
	"context"
	"strings"
	"testing"

	"agent-compose/pkg/images"
	domain "agent-compose/pkg/model"
	"agent-compose/pkg/runtimecache"
	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

type storedSourceStub struct {
	resources []domain.ResolvedResource
	options   domain.ResourceResolveOptions
}

func (s *storedSourceStub) ResolveStoredResources(_ context.Context, options domain.ResourceResolveOptions) ([]domain.ResolvedResource, error) {
	s.options = options
	return append([]domain.ResolvedResource(nil), s.resources...), nil
}

type sandboxSourceStub struct {
	getCalls  int
	listCalls int
	sandbox   *domain.Sandbox
	listed    domain.SandboxListResult
}

func (s *sandboxSourceStub) GetSandbox(context.Context, string) (*domain.Sandbox, error) {
	s.getCalls++
	return s.sandbox, nil
}

func (s *sandboxSourceStub) ListSandboxes(context.Context, domain.SandboxListOptions) (domain.SandboxListResult, error) {
	s.listCalls++
	return s.listed, nil
}

type cacheSourceStub struct {
	calls  int
	result runtimecache.ListResult
}

func (s *cacheSourceStub) ListCaches(context.Context, runtimecache.ListRequest) (runtimecache.ListResult, error) {
	s.calls++
	return s.result, nil
}

type imageBackendStub struct {
	inspect images.InspectResult
	listed  images.ListResult
}

func (s imageBackendStub) ListImages(context.Context, images.ListRequest) (images.ListResult, error) {
	return s.listed, nil
}

func (s imageBackendStub) PullImage(context.Context, images.PullRequest) (images.PullResult, error) {
	return images.PullResult{}, nil
}

func (s imageBackendStub) InspectImage(context.Context, images.InspectRequest) (images.InspectResult, error) {
	return s.inspect, nil
}

func (s imageBackendStub) RemoveImage(context.Context, images.RemoveRequest) (images.RemoveResult, error) {
	return images.RemoveResult{}, nil
}

func TestResolverPrefersExactNameThenExactIDThenPrefix(t *testing.T) {
	prefix := "123456789abc"
	projectID := prefix + strings.Repeat("1", 52)
	runID := prefix + strings.Repeat("2", 52)

	for _, tc := range []struct {
		name      string
		resources []domain.ResolvedResource
		wantKind  domain.ResourceKind
	}{
		{
			name: "name wins over id prefix",
			resources: []domain.ResolvedResource{
				{Kind: domain.ResourceKindProject, MatchType: domain.ResourceMatchName, ID: projectID, Name: prefix},
				{Kind: domain.ResourceKindRun, MatchType: domain.ResourceMatchIDPrefix, ID: runID},
			},
			wantKind: domain.ResourceKindProject,
		},
		{
			name: "exact id wins over id prefix",
			resources: []domain.ResolvedResource{
				{Kind: domain.ResourceKindProject, MatchType: domain.ResourceMatchID, ID: projectID},
				{Kind: domain.ResourceKindRun, MatchType: domain.ResourceMatchIDPrefix, ID: runID},
			},
			wantKind: domain.ResourceKindProject,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			stored := &storedSourceStub{resources: tc.resources}
			resolver := NewResolver(stored, nil, nil, nil)
			result, err := resolver.Resolve(context.Background(), domain.ResourceResolveOptions{
				Ref:   prefix,
				Kinds: []domain.ResourceKind{domain.ResourceKindProject, domain.ResourceKindRun},
			})
			if err != nil {
				t.Fatalf("Resolve returned error: %v", err)
			}
			if len(result.Resources) != 1 || result.Resources[0].Kind != tc.wantKind {
				t.Fatalf("resolved resources = %#v, want one %s", result.Resources, tc.wantKind)
			}
		})
	}
}

func TestResolverPreservesSameRankAmbiguity(t *testing.T) {
	stored := &storedSourceStub{resources: []domain.ResolvedResource{
		{Kind: domain.ResourceKindAgent, MatchType: domain.ResourceMatchName, ID: strings.Repeat("a", 64), Name: "reviewer", ProjectID: "project-b", ProjectName: "b"},
		{Kind: domain.ResourceKindAgent, MatchType: domain.ResourceMatchName, ID: strings.Repeat("b", 64), Name: "reviewer", ProjectID: "project-a", ProjectName: "a"},
	}}
	resolver := NewResolver(stored, nil, nil, nil)
	result, err := resolver.Resolve(context.Background(), domain.ResourceResolveOptions{
		Ref:   "reviewer",
		Kinds: []domain.ResourceKind{domain.ResourceKindAgent},
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if len(result.Resources) != 2 || result.Resources[0].ProjectName != "a" || result.Resources[1].ProjectName != "b" {
		t.Fatalf("resolved resources = %#v, want stable project ambiguity", result.Resources)
	}
}

func TestResolverSkipsOpaqueSourcesForNonIDReference(t *testing.T) {
	stored := &storedSourceStub{}
	sandboxes := &sandboxSourceStub{}
	caches := &cacheSourceStub{}
	resolver := NewResolver(stored, sandboxes, nil, caches)
	result, err := resolver.Resolve(context.Background(), domain.ResourceResolveOptions{
		Ref:   "human-name",
		Kinds: []domain.ResourceKind{domain.ResourceKindSandbox, domain.ResourceKindCache},
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if len(result.Resources) != 0 || sandboxes.getCalls != 0 || sandboxes.listCalls != 0 || caches.calls != 0 {
		t.Fatalf("result/calls = %#v / sandbox(%d,%d) / cache(%d)", result, sandboxes.getCalls, sandboxes.listCalls, caches.calls)
	}
}

func TestResolverExactNameSkipsLowerPriorityOpaqueIDScans(t *testing.T) {
	ref := "123456789abc"
	stored := &storedSourceStub{resources: []domain.ResolvedResource{{
		Kind: domain.ResourceKindProject, MatchType: domain.ResourceMatchName, Name: ref, ID: ref + strings.Repeat("a", 52),
	}}}
	sandboxes := &sandboxSourceStub{}
	caches := &cacheSourceStub{}
	resolver := NewResolver(stored, sandboxes, nil, caches)
	result, err := resolver.Resolve(context.Background(), domain.ResourceResolveOptions{
		Ref:   ref,
		Kinds: []domain.ResourceKind{domain.ResourceKindProject, domain.ResourceKindSandbox, domain.ResourceKindCache},
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if len(result.Resources) != 1 || sandboxes.getCalls != 0 || sandboxes.listCalls != 0 || caches.calls != 0 {
		t.Fatalf("result/calls = %#v / sandbox(%d,%d) / cache(%d)", result, sandboxes.getCalls, sandboxes.listCalls, caches.calls)
	}
}

func TestResolverFindsAllImageIDPrefixMatches(t *testing.T) {
	prefix := "abcdef123456"
	firstID := "sha256:" + prefix + strings.Repeat("a", 52)
	secondID := "sha256:" + prefix + strings.Repeat("b", 52)
	backend := imageBackendStub{
		inspect: images.InspectResult{Image: &agentcomposev2.Image{ImageId: firstID}},
		listed: images.ListResult{Images: []*agentcomposev2.Image{
			{ImageId: firstID},
			{ImageId: secondID},
		}},
	}
	resolver := NewResolver(&storedSourceStub{}, nil, backend, nil)
	result, err := resolver.Resolve(context.Background(), domain.ResourceResolveOptions{
		Ref:   prefix,
		Kinds: []domain.ResourceKind{domain.ResourceKindImage},
	})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if len(result.Resources) != 2 {
		t.Fatalf("resolved resources = %#v, want two image prefix matches", result.Resources)
	}
}

func TestResolverReportsCrossTypeNameAmbiguity(t *testing.T) {
	stored := &storedSourceStub{resources: []domain.ResolvedResource{{
		Kind: domain.ResourceKindVolume, MatchType: domain.ResourceMatchName, Name: "shared", InspectRef: "shared",
	}}}
	backend := imageBackendStub{inspect: images.InspectResult{Image: &agentcomposev2.Image{
		ImageId: strings.Repeat("c", 64), ImageRef: "shared",
	}}}
	resolver := NewResolver(stored, nil, backend, nil)
	result, err := resolver.Resolve(context.Background(), domain.ResourceResolveOptions{Ref: "shared"})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if len(result.Resources) != 2 || result.Resources[0].Kind != domain.ResourceKindImage || result.Resources[1].Kind != domain.ResourceKindVolume {
		t.Fatalf("resolved resources = %#v, want image and volume ambiguity", result.Resources)
	}
}
