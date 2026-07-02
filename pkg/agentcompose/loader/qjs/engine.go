package qjs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/fastschema/qjs"
	"github.com/samber/do/v2"

	"agent-compose/pkg/agentcompose/event"
	"agent-compose/pkg/agentcompose/loader"
)

type TopicEventRecord = event.TopicEventRecord

type LoaderAgentRequest = loader.LoaderAgentRequest

type LoaderAgentResult = loader.LoaderAgentResult

type LoaderCommandRequest = loader.LoaderCommandRequest

type LoaderCommandResult = loader.LoaderCommandResult

type LoaderLLMRequest = loader.LoaderLLMRequest

type LoaderLLMResult = loader.LoaderLLMResult

type LoaderTrigger = loader.LoaderTrigger

type SessionEnvVar = loader.SessionEnvVar

type LoaderHost = loader.LoaderHost

type LoaderValidationResult = loader.LoaderValidationResult

type LoaderExecutionRequest = loader.LoaderExecutionRequest

type LoaderExecutionResult = loader.LoaderExecutionResult

type LoaderEngine interface {
	Validate(ctx context.Context, runtime, script string) (LoaderValidationResult, error)
	Execute(ctx context.Context, request LoaderExecutionRequest, host LoaderHost) (LoaderExecutionResult, error)
}

type QJSLoaderEngine struct{}

func NewLoaderEngine(do.Injector) (LoaderEngine, error) {
	return &QJSLoaderEngine{}, nil
}

func (e *QJSLoaderEngine) Validate(ctx context.Context, runtime, script string) (LoaderValidationResult, error) {
	result, err := e.execute(ctx, LoaderExecutionRequest{Runtime: runtime, Script: script}, nil, true)
	if err != nil {
		return LoaderValidationResult{}, err
	}
	return LoaderValidationResult{Triggers: result.Triggers, Warnings: result.Warnings}, nil
}

func (e *QJSLoaderEngine) Execute(ctx context.Context, request LoaderExecutionRequest, host LoaderHost) (LoaderExecutionResult, error) {
	return e.execute(ctx, request, host, false)
}

func (e *QJSLoaderEngine) execute(ctx context.Context, request LoaderExecutionRequest, host LoaderHost, validateOnly bool) (LoaderExecutionResult, error) {
	runtimeName, err := loader.NormalizeRuntime(request.Runtime)
	if err != nil {
		return LoaderExecutionResult{}, err
	}
	if runtimeName != loader.LoaderRuntimeScheduler {
		return LoaderExecutionResult{}, fmt.Errorf("unsupported loader runtime %q", runtimeName)
	}
	if strings.TrimSpace(request.Script) == "" {
		return LoaderExecutionResult{}, fmt.Errorf("loader script is required")
	}

	rt, err := qjs.New(qjs.Option{
		Context:          ctx,
		MemoryLimit:      64 << 20,
		MaxExecutionTime: loaderEngineMaxExecutionTime(ctx),
	})
	if err != nil {
		return LoaderExecutionResult{}, fmt.Errorf("create qjs runtime: %w", err)
	}
	defer rt.Close()

	jsctx := rt.Context()
	state := &loaderExecutionState{
		ctx:           ctx,
		host:          host,
		registrations: make([]loaderRegistration, 0),
		seenIDs:       make(map[string]struct{}),
	}

	if _, err = e.installRuntime(jsctx, state); err != nil {
		return LoaderExecutionResult{}, err
	}

	evalResult, err := jsctx.Eval("loader.js", qjs.Code(request.Script), qjs.FlagAsync())
	if err != nil {
		state.freeCallbacks()
		return LoaderExecutionResult{}, fmt.Errorf("evaluate loader script: %w", err)
	}
	if evalResult != nil {
		if evalResult.IsPromise() {
			if _, err := evalResult.Await(); err != nil {
				state.freeCallbacks()
				return LoaderExecutionResult{}, fmt.Errorf("await loader script: %w", err)
			}
		}
	}

	warnings := make([]string, 0)
	if len(state.registrations) == 0 {
		mainFn := jsctx.Global().GetPropertyStr("main")
		hasMain := mainFn.IsFunction()
		if !hasMain {
			warnings = append(warnings, "script does not register any trigger and does not define main()")
		}
	}

	result := LoaderExecutionResult{
		Triggers: state.triggers(),
		Warnings: warnings,
	}
	if validateOnly {
		state.freeCallbacks()
		return result, nil
	}
	if host == nil {
		state.freeCallbacks()
		return LoaderExecutionResult{}, fmt.Errorf("loader host is required for execution")
	}

	payloadValue, err := payloadValueFromJSON(jsctx, request.PayloadJSON)
	if err != nil {
		state.freeCallbacks()
		return LoaderExecutionResult{}, err
	}

	executed, err := e.executeRequestedHandler(jsctx, state, request.Trigger, payloadValue)
	if err != nil {
		state.freeCallbacks()
		return LoaderExecutionResult{}, err
	}
	if executed != nil {
		if executed.IsPromise() {
			awaited, err := executed.Await()
			if err != nil {
				state.freeCallbacks()
				return LoaderExecutionResult{}, fmt.Errorf("await loader handler: %w", err)
			}
			executed = awaited
		}
		if jsonResult, ok, err := loaderResultJSON(executed); err != nil {
			state.freeCallbacks()
			return LoaderExecutionResult{}, err
		} else if ok {
			result.ResultJSON = jsonResult
		}
	}
	if err := ctx.Err(); err != nil {
		state.freeCallbacks()
		return LoaderExecutionResult{}, err
	}
	state.freeCallbacks()
	return result, nil
}

func loaderEngineMaxExecutionTime(ctx context.Context) int {
	const defaultMaxExecutionTimeMs = int((60 * time.Minute) / time.Millisecond)
	deadline, ok := ctx.Deadline()
	if !ok {
		return defaultMaxExecutionTimeMs
	}
	remaining := time.Until(deadline)
	if remaining <= 0 {
		return 1
	}
	remainingMs := int(remaining / time.Millisecond)
	if remainingMs < 1 {
		return 1
	}
	return remainingMs
}

func LoaderEngineMaxExecutionTime(ctx context.Context) int {
	return loaderEngineMaxExecutionTime(ctx)
}
