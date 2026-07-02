# Architecture Metrics Baseline
Generated: 2026-07-02T01:49:04Z
Branch: `refactor/hardening-metrics`
Commit: `6245bc56d201`
Scope:
- `./cmd/...`
- `./internal/...`
- `./pkg/agentcompose/...`
## Package Size
| Go LOC | Go files | Test files | Package |
| ---: | ---: | ---: | --- |
| 34543 | 57 | 38 | `agent-compose/pkg/agentcompose` |
| 2889 | 1 | 5 | `agent-compose/cmd/agent-compose` |
| 2860 | 8 | 0 | `agent-compose/pkg/agentcompose/loader` |
| 2438 | 4 | 0 | `agent-compose/pkg/agentcompose/llm` |
| 2054 | 1 | 0 | `agent-compose/internal/cli/compose` |
| 1600 | 1 | 0 | `agent-compose/pkg/agentcompose/loader/qjs` |
| 1357 | 3 | 0 | `agent-compose/pkg/agentcompose/event` |
| 1100 | 5 | 0 | `agent-compose/pkg/agentcompose/image` |
| 1065 | 2 | 0 | `agent-compose/pkg/agentcompose/webhook` |
| 995 | 6 | 0 | `agent-compose/pkg/agentcompose/session` |
| 817 | 5 | 0 | `agent-compose/pkg/agentcompose/run` |
| 722 | 1 | 0 | `agent-compose/pkg/agentcompose/workspace` |
| 669 | 4 | 0 | `agent-compose/pkg/agentcompose/project` |
| 651 | 1 | 0 | `agent-compose/pkg/agentcompose/store/sessionfs` |
| 538 | 2 | 1 | `agent-compose/internal/daemon` |
| 527 | 4 | 1 | `agent-compose/pkg/agentcompose/store/sqlite` |
| 327 | 1 | 0 | `agent-compose/pkg/agentcompose/agentdef` |
| 258 | 3 | 0 | `agent-compose/pkg/agentcompose/runtime` |
| 226 | 1 | 0 | `agent-compose/pkg/agentcompose/transport/http/workspace` |
| 140 | 1 | 0 | `agent-compose/pkg/agentcompose/transport/http` |
| 130 | 1 | 0 | `agent-compose/pkg/agentcompose/transport/connectv2` |
| 87 | 1 | 0 | `agent-compose/internal/cli` |
| 81 | 1 | 0 | `agent-compose/pkg/agentcompose/configsvc` |
| 48 | 1 | 0 | `agent-compose/pkg/agentcompose/transport/connectv1` |
| 32 | 1 | 0 | `agent-compose/pkg/agentcompose/transport/http/runtimellm` |
| 23 | 1 | 0 | `agent-compose/pkg/agentcompose/app` |
## Largest Production Files
| Lines | File |
| ---: | --- |
| 2054 | `internal/cli/compose/compose.go` |
| 1706 | `pkg/agentcompose/project_service.go` |
| 1600 | `pkg/agentcompose/loader/qjs/engine.go` |
| 1503 | `pkg/agentcompose/service.go` |
| 1465 | `pkg/agentcompose/loader/store.go` |
| 1227 | `pkg/agentcompose/llm/config.go` |
| 1217 | `pkg/agentcompose/loader_manager.go` |
| 1001 | `pkg/agentcompose/exec.go` |
| 999 | `pkg/agentcompose/event/store.go` |
| 930 | `pkg/agentcompose/llm_config.go` |
... (154 lines truncated)
