package app

import (
	capdomain "agent-compose/internal/capability"
	execdomain "agent-compose/internal/exec"
	loaderdomain "agent-compose/internal/loader"
	filestore "agent-compose/internal/persistence/filestore"
	sessiondomain "agent-compose/internal/session"
	llmdomain "agent-compose/pkg/llm"

	"github.com/samber/do/v2"
)

func registerProviders(di do.Injector) {
	do.Provide(di, NewStore)
	do.Provide(di, NewConfigStore)
	do.Provide(di, NewRuntimeProvider)
	do.Provide(di, func(i do.Injector) (sessiondomain.Store, error) {
		return do.MustInvoke[*Store](i), nil
	})
	do.Provide(di, func(i do.Injector) (sessiondomain.ConfigStore, error) {
		return do.MustInvoke[*ConfigStore](i), nil
	})
	do.Provide(di, func(i do.Injector) (filestore.ConfigStore, error) {
		return do.MustInvoke[*ConfigStore](i), nil
	})
	do.Provide(di, func(i do.Injector) (execdomain.Store, error) {
		return do.MustInvoke[*Store](i), nil
	})
	do.Provide(di, func(i do.Injector) (execdomain.ConfigStore, error) {
		return do.MustInvoke[*ConfigStore](i), nil
	})
	do.Provide(di, func(i do.Injector) (execdomain.SessionStreamBroker, error) {
		return do.MustInvoke[*SessionStreamBroker](i), nil
	})
	do.Provide(di, func(i do.Injector) (loaderdomain.ConfigStore, error) {
		return do.MustInvoke[*ConfigStore](i), nil
	})
	do.Provide(di, func(i do.Injector) (loaderdomain.SessionRPCBridge, error) {
		return do.MustInvoke[*SessionRPCBridge](i), nil
	})
	do.Provide(di, func(i do.Injector) (llmdomain.ConfigStore, error) {
		return do.MustInvoke[*ConfigStore](i), nil
	})
	do.Provide(di, func(i do.Injector) (capdomain.CapabilityProvider, error) {
		return do.MustInvoke[capabilityIntegration](i), nil
	})
	do.Provide(di, func(i do.Injector) (CapabilityProvider, error) {
		return do.MustInvoke[capabilityIntegration](i), nil
	})
	do.Provide(di, NewDriver)
	do.Provide(di, NewExecutor)
	do.Provide(di, NewLLMClient)
	do.Provide(di, NewCapabilityProvider)
	do.Provide(di, NewCapProxyServer)
	do.Provide(di, NewLoaderBus)
	do.Provide(di, NewSessionStreamBroker)
	do.Provide(di, NewDashboardOverviewAggregator)
	do.Provide(di, NewDashboardOverviewHub)
	do.Provide(di, NewLoaderEngine)
	do.Provide(di, NewSessionRPCBridge)
	do.Provide(di, NewLoaderManager)
	do.Provide(di, NewService)
	do.Provide(di, NewProjectService)
}
