package api

import (
	"context"
	"errors"
	"testing"

	"connectrpc.com/connect"

	domain "agent-compose/pkg/model"
	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

type resourceResolverStub struct {
	options domain.ResourceResolveOptions
	result  domain.ResourceResolveResult
	err     error
}

func (s *resourceResolverStub) Resolve(_ context.Context, options domain.ResourceResolveOptions) (domain.ResourceResolveResult, error) {
	s.options = options
	return s.result, s.err
}

func TestResourceHandlerResolveResource(t *testing.T) {
	resolver := &resourceResolverStub{result: domain.ResourceResolveResult{
		Resources: []domain.ResolvedResource{{
			Kind: domain.ResourceKindVolume, MatchType: domain.ResourceMatchName, Name: "cache-data", InspectRef: "cache-data",
		}},
		Warnings: []string{"image resolver unavailable"},
	}}
	handler := NewResourceHandler(resolver)
	response, err := handler.ResolveResource(context.Background(), connect.NewRequest(&agentcomposev2.ResolveResourceRequest{
		Ref:   "cache-data",
		Kinds: []agentcomposev2.ResourceKind{agentcomposev2.ResourceKind_RESOURCE_KIND_VOLUME},
	}))
	if err != nil {
		t.Fatalf("ResolveResource returned error: %v", err)
	}
	if resolver.options.Ref != "cache-data" || len(resolver.options.Kinds) != 1 || resolver.options.Kinds[0] != domain.ResourceKindVolume {
		t.Fatalf("resolver options = %#v", resolver.options)
	}
	resources := response.Msg.GetResources()
	if len(resources) != 1 || resources[0].GetKind() != agentcomposev2.ResourceKind_RESOURCE_KIND_VOLUME || resources[0].GetId() != "" || resources[0].GetInspectRef() != "cache-data" {
		t.Fatalf("response resources = %#v", resources)
	}
	if len(response.Msg.GetWarnings()) != 1 {
		t.Fatalf("response warnings = %#v", response.Msg.GetWarnings())
	}
}

func TestResourceHandlerValidationAndResolverErrors(t *testing.T) {
	handler := NewResourceHandler(&resourceResolverStub{})
	if _, err := handler.ResolveResource(context.Background(), connect.NewRequest(&agentcomposev2.ResolveResourceRequest{})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("empty ref error = %v, want invalid argument", err)
	}
	if _, err := handler.ResolveResource(context.Background(), connect.NewRequest(&agentcomposev2.ResolveResourceRequest{
		Ref: "ref", Kinds: []agentcomposev2.ResourceKind{agentcomposev2.ResourceKind_RESOURCE_KIND_UNSPECIFIED},
	})); connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Fatalf("unspecified kind error = %v, want invalid argument", err)
	}
	resolver := &resourceResolverStub{err: errors.New("lookup failed")}
	if _, err := NewResourceHandler(resolver).ResolveResource(context.Background(), connect.NewRequest(&agentcomposev2.ResolveResourceRequest{Ref: "ref"})); connect.CodeOf(err) != connect.CodeInternal {
		t.Fatalf("resolver error = %v, want internal", err)
	}
	if _, err := NewResourceHandler(nil).ResolveResource(context.Background(), connect.NewRequest(&agentcomposev2.ResolveResourceRequest{Ref: "ref"})); connect.CodeOf(err) != connect.CodeUnavailable {
		t.Fatalf("nil resolver error = %v, want unavailable", err)
	}
}
