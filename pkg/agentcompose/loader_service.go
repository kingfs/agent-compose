package agentcompose

import (
	"context"
	"fmt"
	"strings"
	"time"

	"connectrpc.com/connect"
	"google.golang.org/protobuf/types/known/emptypb"

	loaderpkg "agent-compose/pkg/agentcompose/loader"
	agentcomposev1 "agent-compose/proto/agentcompose/v1"
)

func (s *Service) loaderService() *loaderpkg.Service {
	return &loaderpkg.Service{
		Manager:             s.loaders,
		Repository:          s.configDB,
		ResolveDefaultAgent: s.resolveLoaderDefaultAgent,
	}
}

func (s *Service) ValidateLoader(ctx context.Context, req *connect.Request[agentcomposev1.ValidateLoaderRequest]) (*connect.Response[agentcomposev1.ValidateLoaderResponse], error) {
	return s.loaderService().ValidateLoader(ctx, req)
}

func (s *Service) ListLoaders(ctx context.Context, req *connect.Request[emptypb.Empty]) (*connect.Response[agentcomposev1.ListLoadersResponse], error) {
	return s.loaderService().ListLoaders(ctx, req)
}

func (s *Service) GetLoader(ctx context.Context, req *connect.Request[agentcomposev1.LoaderIDRequest]) (*connect.Response[agentcomposev1.LoaderResponse], error) {
	return s.loaderService().GetLoader(ctx, req)
}

func (s *Service) CreateLoader(ctx context.Context, req *connect.Request[agentcomposev1.CreateLoaderRequest]) (*connect.Response[agentcomposev1.LoaderResponse], error) {
	return s.loaderService().CreateLoader(ctx, req)
}

func (s *Service) UpdateLoader(ctx context.Context, req *connect.Request[agentcomposev1.UpdateLoaderRequest]) (*connect.Response[agentcomposev1.LoaderResponse], error) {
	return s.loaderService().UpdateLoader(ctx, req)
}

func (s *Service) resolveLoaderDefaultAgent(ctx context.Context, agentID, provider string) (string, error) {
	agentID = strings.TrimSpace(agentID)
	if agentID == "" {
		return provider, nil
	}
	agent, err := s.configDB.GetAgentDefinition(ctx, agentID)
	if err != nil {
		return "", err
	}
	if !agent.Enabled {
		return "", fmt.Errorf("agent definition %s is disabled", agentID)
	}
	if strings.TrimSpace(provider) != "" && normalizeAgentKind(provider) == "" {
		return "", fmt.Errorf("loader default agent provider %q is not supported", provider)
	}
	return agent.Provider, nil
}

func (s *Service) DeleteLoader(ctx context.Context, req *connect.Request[agentcomposev1.LoaderIDRequest]) (*connect.Response[emptypb.Empty], error) {
	return s.loaderService().DeleteLoader(ctx, req)
}

func (s *Service) SetLoaderEnabled(ctx context.Context, req *connect.Request[agentcomposev1.SetLoaderEnabledRequest]) (*connect.Response[agentcomposev1.LoaderResponse], error) {
	return s.loaderService().SetLoaderEnabled(ctx, req)
}

func (s *Service) SetLoaderTriggerEnabled(ctx context.Context, req *connect.Request[agentcomposev1.SetLoaderTriggerEnabledRequest]) (*connect.Response[agentcomposev1.LoaderResponse], error) {
	return s.loaderService().SetLoaderTriggerEnabled(ctx, req)
}

func (s *Service) RunLoaderNow(ctx context.Context, req *connect.Request[agentcomposev1.RunLoaderNowRequest]) (*connect.Response[agentcomposev1.LoaderRunResponse], error) {
	return s.loaderService().RunLoaderNow(ctx, req)
}

func parseLoaderRunTimeout(raw string) (time.Duration, error) {
	return loaderpkg.ParseRunTimeout(raw)
}

func (s *Service) ListLoaderRuns(ctx context.Context, req *connect.Request[agentcomposev1.ListLoaderRunsRequest]) (*connect.Response[agentcomposev1.ListLoaderRunsResponse], error) {
	return s.loaderService().ListLoaderRuns(ctx, req)
}

func (s *Service) GetLoaderRun(ctx context.Context, req *connect.Request[agentcomposev1.LoaderRunIDRequest]) (*connect.Response[agentcomposev1.LoaderRunResponse], error) {
	return s.loaderService().GetLoaderRun(ctx, req)
}

func (s *Service) ListLoaderEvents(ctx context.Context, req *connect.Request[agentcomposev1.ListLoaderEventsRequest]) (*connect.Response[agentcomposev1.ListLoaderEventsResponse], error) {
	return s.loaderService().ListLoaderEvents(ctx, req)
}
