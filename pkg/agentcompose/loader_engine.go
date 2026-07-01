package agentcompose

import (
	"context"

	"agent-compose/pkg/agentcompose/loader/qjs"

	"github.com/samber/do/v2"
)

type LoaderHost = qjs.LoaderHost
type LoaderValidationResult = qjs.LoaderValidationResult
type LoaderExecutionRequest = qjs.LoaderExecutionRequest
type LoaderExecutionResult = qjs.LoaderExecutionResult
type LoaderEngine = qjs.LoaderEngine
type QJSLoaderEngine = qjs.QJSLoaderEngine

func NewLoaderEngine(di do.Injector) (LoaderEngine, error) {
	return qjs.NewLoaderEngine(di)
}

func loaderEngineMaxExecutionTime(ctx context.Context) int {
	return qjs.LoaderEngineMaxExecutionTime(ctx)
}
