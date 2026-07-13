package api

import (
	"context"
	"fmt"
	"strings"

	"connectrpc.com/connect"

	domain "agent-compose/pkg/model"
	agentcomposev2 "agent-compose/proto/agentcompose/v2"
)

type ResourceResolver interface {
	Resolve(context.Context, domain.ResourceResolveOptions) (domain.ResourceResolveResult, error)
}

type ResourceHandler struct {
	resolver ResourceResolver
}

func NewResourceHandler(resolver ResourceResolver) *ResourceHandler {
	return &ResourceHandler{resolver: resolver}
}

func (h *ResourceHandler) ResolveResource(ctx context.Context, req *connect.Request[agentcomposev2.ResolveResourceRequest]) (*connect.Response[agentcomposev2.ResolveResourceResponse], error) {
	if h == nil || h.resolver == nil {
		return nil, connect.NewError(connect.CodeUnavailable, fmt.Errorf("resource resolver is unavailable"))
	}
	ref := strings.TrimSpace(req.Msg.GetRef())
	if ref == "" {
		return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("resource ref is required"))
	}
	kinds := make([]domain.ResourceKind, 0, len(req.Msg.GetKinds()))
	for _, kind := range req.Msg.GetKinds() {
		mapped, ok := ResourceKindFromProto(kind)
		if !ok {
			return nil, connect.NewError(connect.CodeInvalidArgument, fmt.Errorf("unsupported resource kind %s", kind.String()))
		}
		kinds = append(kinds, mapped)
	}
	result, err := h.resolver.Resolve(ctx, domain.ResourceResolveOptions{Ref: ref, Kinds: kinds})
	if err != nil {
		return nil, connect.NewError(connect.CodeInternal, err)
	}
	response := &agentcomposev2.ResolveResourceResponse{Warnings: append([]string(nil), result.Warnings...)}
	for _, resource := range result.Resources {
		response.Resources = append(response.Resources, ResolvedResourceToProto(resource))
	}
	return connect.NewResponse(response), nil
}

func ResourceKindFromProto(kind agentcomposev2.ResourceKind) (domain.ResourceKind, bool) {
	switch kind {
	case agentcomposev2.ResourceKind_RESOURCE_KIND_PROJECT:
		return domain.ResourceKindProject, true
	case agentcomposev2.ResourceKind_RESOURCE_KIND_AGENT:
		return domain.ResourceKindAgent, true
	case agentcomposev2.ResourceKind_RESOURCE_KIND_RUN:
		return domain.ResourceKindRun, true
	case agentcomposev2.ResourceKind_RESOURCE_KIND_SANDBOX:
		return domain.ResourceKindSandbox, true
	case agentcomposev2.ResourceKind_RESOURCE_KIND_IMAGE:
		return domain.ResourceKindImage, true
	case agentcomposev2.ResourceKind_RESOURCE_KIND_CACHE:
		return domain.ResourceKindCache, true
	case agentcomposev2.ResourceKind_RESOURCE_KIND_VOLUME:
		return domain.ResourceKindVolume, true
	default:
		return "", false
	}
}

func ResourceKindToProto(kind domain.ResourceKind) agentcomposev2.ResourceKind {
	switch kind {
	case domain.ResourceKindProject:
		return agentcomposev2.ResourceKind_RESOURCE_KIND_PROJECT
	case domain.ResourceKindAgent:
		return agentcomposev2.ResourceKind_RESOURCE_KIND_AGENT
	case domain.ResourceKindRun:
		return agentcomposev2.ResourceKind_RESOURCE_KIND_RUN
	case domain.ResourceKindSandbox:
		return agentcomposev2.ResourceKind_RESOURCE_KIND_SANDBOX
	case domain.ResourceKindImage:
		return agentcomposev2.ResourceKind_RESOURCE_KIND_IMAGE
	case domain.ResourceKindCache:
		return agentcomposev2.ResourceKind_RESOURCE_KIND_CACHE
	case domain.ResourceKindVolume:
		return agentcomposev2.ResourceKind_RESOURCE_KIND_VOLUME
	default:
		return agentcomposev2.ResourceKind_RESOURCE_KIND_UNSPECIFIED
	}
}

func ResourceMatchTypeToProto(matchType domain.ResourceMatchType) agentcomposev2.ResourceMatchType {
	switch matchType {
	case domain.ResourceMatchName:
		return agentcomposev2.ResourceMatchType_RESOURCE_MATCH_TYPE_NAME
	case domain.ResourceMatchID:
		return agentcomposev2.ResourceMatchType_RESOURCE_MATCH_TYPE_ID
	case domain.ResourceMatchIDPrefix:
		return agentcomposev2.ResourceMatchType_RESOURCE_MATCH_TYPE_ID_PREFIX
	default:
		return agentcomposev2.ResourceMatchType_RESOURCE_MATCH_TYPE_UNSPECIFIED
	}
}

func ResolvedResourceToProto(resource domain.ResolvedResource) *agentcomposev2.ResolvedResource {
	return &agentcomposev2.ResolvedResource{
		Kind:        ResourceKindToProto(resource.Kind),
		MatchType:   ResourceMatchTypeToProto(resource.MatchType),
		Id:          resource.ID,
		ShortId:     resource.ShortID,
		Name:        resource.Name,
		ProjectId:   resource.ProjectID,
		ProjectName: resource.ProjectName,
		InspectRef:  resource.InspectRef,
	}
}
