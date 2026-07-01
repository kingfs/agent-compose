# Module Split Refactor Plan

## Purpose

This document is the controlling plan for the large module split refactor of agent-compose.

The goal is to make the codebase module boundaries clear while preserving existing behavior. The refactor must not change public API contracts, persistence formats, runtime contracts, loader script behavior, environment variable names, or business logic semantics.

All implementation agents must follow this plan. Do not invent additional refactor scope, do not turn assigned work into small cleanup tasks, and do not make opportunistic improvements that are not required for the assigned module split.

## Non-Negotiable Constraints

- Keep existing Connect APIs compatible.
- Keep existing HTTP routes compatible.
- Keep proto files compatible unless a separate approved API task is created.
- Keep SQLite schema and session filesystem layout compatible unless a separate approved migration task is created.
- Keep runtime guest contracts compatible.
- Keep loader script API and loader execution semantics compatible.
- Keep configuration and environment variable names compatible.
- Preserve existing tests and add only focused tests needed to prove moved behavior still works.
- Prefer compatibility wrappers during migration so branches can merge incrementally.
- Do not mix broad business changes with module movement.

## Owner Role

The refactor owner is responsible for:

- Maintaining this plan and the progress document.
- Assigning module-level work to agents.
- Creating and naming git worktree branches.
- Ensuring each agent stays inside the assigned module boundary.
- Reviewing branch scope before merge.
- Merging completed work into the module split integration branch.
- Running integration tests after merges.
- Cleaning merged worktrees.
- Updating progress after every material state change.

Implementation agents are responsible for:

- Working only on the assigned module task.
- Preserving behavior.
- Reporting blockers early.
- Avoiding unrelated cleanup, renaming, formatting churn, or architecture changes.
- Keeping branch changes reviewable at module level.

## Target Package Layout

The target package layout is:

```text
pkg/agentcompose/
  app/                 # DI wiring, route registration, lifecycle startup
  transport/
    connectv1/         # v1 Connect handlers and proto mapping
    connectv2/         # v2 Connect handlers and proto mapping
    http/              # Echo HTTP routes: proxy, webhook, workspace, LLM facade
  domain/
    model/             # only stable cross-module domain types, kept minimal
  session/             # session use cases, session model, streams, RPC bridge ports
  runtime/             # runtime provider and session driver ports/adapters
  exec/                # cell, command, and agent execution
  loader/              # loader model, manager, scheduler, run execution
  project/             # project apply, managed resources, project records
  run/                 # project run lifecycle
  event/               # topic events, event dispatching, event repository ports
  webhook/             # webhook request handling and queueing
  workspace/           # workspace model and workspace business logic
  llm/                 # LLM client, config, runtime facade domain logic
  image/               # image service, backends, image ensure logic
  capability/          # app-level capability integration
  store/
    sqlite/            # concrete SQLite repositories and schema management
    sessionfs/         # filesystem-backed session persistence
```

Existing independent packages should remain independent unless a later approved task says otherwise:

```text
pkg/auth
pkg/capability
pkg/capproxy
pkg/compose
pkg/config
pkg/dbo
pkg/driver
pkg/fxgo
pkg/health
pkg/imagecache
```

## Design Rules

### Dependency Direction

Use this dependency direction:

```text
transport -> application module -> module-owned ports -> infrastructure implementation
```

Concrete implications:

- Transport handlers may import proto/connect/echo.
- Core module services should not expose proto/connect/echo types.
- Repository interfaces belong to the consuming module.
- SQLite implementations belong under `pkg/agentcompose/store/sqlite`.
- DI container usage belongs in `pkg/agentcompose/app` and top-level setup only.
- Avoid passing `do.Injector` into module services.

### Compatibility Wrappers

During migration, compatibility wrappers are allowed in `pkg/agentcompose` so existing code can continue to compile while branches are merged.

Wrappers must be temporary and thin:

- Delegate to the new module.
- Avoid duplicating logic.
- Avoid adding new behavior.
- Be removed in the cleanup wave after transport and store split are complete.

### Mapping Rules

- Proto mapping belongs in transport packages.
- Database row scanning and SQL mapping belongs in store packages.
- Domain normalization belongs in the owning module.
- Cross-module DTOs should be kept small and explicit.

## Worktree and Branch Model

Base branch:

```bash
refactor/module-split-base
```

Integration branch:

```bash
refactor/module-split-integration
```

Each module task gets a dedicated worktree and branch:

```bash
git worktree add ../agent-compose-refactor-<module> refactor/<module>-module
```

No agent should share a worktree with another agent.

## Phase Plan

### Phase 0: Baseline and Governance

Branch:

```text
refactor/module-split-base
```

Scope:

- Add target directory skeleton where useful.
- Add this plan.
- Add the progress tracker.
- Establish baseline tests.
- Do not move production logic in this phase.

Exit criteria:

- Plan and progress documents exist.
- Baseline branch is ready for module worktrees.
- Existing dirty user changes are not overwritten.

### Phase 1: Parallel Module Splits

These tasks are module-sized and can run in parallel after Phase 0.

#### Task 1: Session Module

Branch:

```text
refactor/session-module
```

Primary files:

- `pkg/agentcompose/model.go` session-related parts
- `pkg/agentcompose/store.go`
- `pkg/agentcompose/session_rpc_bridge.go`
- `pkg/agentcompose/session_driver.go`
- `pkg/agentcompose/session_stream.go`
- `pkg/agentcompose/session_list.go`
- `pkg/agentcompose/session_proto.go`
- `pkg/agentcompose/session_reconcile.go`

Target packages:

- `pkg/agentcompose/session`
- `pkg/agentcompose/runtime`
- `pkg/agentcompose/store/sessionfs`

Scope:

- Extract session model, session lifecycle service, session stream broker, and filesystem session persistence.
- Keep public API behavior unchanged through existing facade methods.
- Define runtime/session driver ports without changing driver behavior.

Out of scope:

- Loader behavior changes.
- Project behavior changes.
- Transport handler rewrite.

Suggested validation:

```bash
go test ./pkg/agentcompose ./pkg/driver
```

#### Task 2: Loader Module

Branch:

```text
refactor/loader-module
```

Primary files:

- `pkg/agentcompose/loader_model.go`
- `pkg/agentcompose/loader_schedule.go`
- `pkg/agentcompose/loader_engine.go`
- `pkg/agentcompose/loader_manager.go`
- `pkg/agentcompose/loader_run_executor.go`
- `pkg/agentcompose/loader_session_runner.go`
- `pkg/agentcompose/loader_event_dispatcher.go`
- `pkg/agentcompose/loader_events.go`
- `pkg/agentcompose/loader_bus.go`
- `pkg/agentcompose/loader_service.go`
- `pkg/agentcompose/loader_store.go`

Target packages:

- `pkg/agentcompose/loader`
- `pkg/agentcompose/loader/qjs`

Scope:

- Extract loader model, loader manager, scheduler, run executor, session runner, event dispatcher, bus, and QJS engine.
- Define `loader.Repository` and runtime/session ports consumed by loader.
- Keep existing loader DB schema and loader script API unchanged.

Out of scope:

- Project managed scheduler logic changes.
- SQLite store physical split, except thin adapters needed to compile.
- Loader runtime semantics changes.

Suggested validation:

```bash
go test ./pkg/agentcompose -run 'Loader|Webhook|Event'
```

#### Task 3: Project and Run Modules

Branch:

```text
refactor/project-run-module
```

Primary files:

- `pkg/agentcompose/project_service.go`
- `pkg/agentcompose/project_store.go`
- `pkg/agentcompose/project_schema.go`
- `pkg/agentcompose/project_session.go`
- `pkg/agentcompose/project_agent_runner.go`
- `pkg/agentcompose/project_down.go`
- `pkg/agentcompose/run_service.go`
- `pkg/agentcompose/run_coordinator.go`
- `pkg/agentcompose/run_preparation.go`
- `pkg/agentcompose/run_session.go`

Target packages:

- `pkg/agentcompose/project`
- `pkg/agentcompose/run`

Scope:

- Extract project apply, managed agent reconciliation, managed scheduler reconciliation, project persistence ports, and run lifecycle.
- Keep project apply semantics identical.
- Interact with loader through a narrow managed-loader port.

Out of scope:

- Compose spec changes.
- Loader manager internals, except port use.
- API response shape changes.

Suggested validation:

```bash
go test ./pkg/agentcompose -run 'Project|Run|Compose'
```

#### Task 4: Event and Webhook Modules

Branch:

```text
refactor/event-webhook-module
```

Primary files:

- `pkg/agentcompose/topic_event_model.go`
- `pkg/agentcompose/topic_event_store.go`
- `pkg/agentcompose/event_dispatcher.go`
- `pkg/agentcompose/webhook.go`
- `pkg/agentcompose/webhook_queue.go`

Target packages:

- `pkg/agentcompose/event`
- `pkg/agentcompose/webhook`

Scope:

- Extract topic event model, repository ports, event dispatcher, webhook handler, and webhook queue.
- Keep event table schema, dispatch status semantics, idempotency behavior, and webhook route behavior unchanged.

Out of scope:

- Loader event execution logic.
- HTTP transport package migration beyond required wrappers.

Suggested validation:

```bash
go test ./pkg/agentcompose -run 'Event|Webhook|Topic'
```

#### Task 5: LLM Module

Branch:

```text
refactor/llm-module
```

Primary files:

- `pkg/agentcompose/llm_client.go`
- `pkg/agentcompose/llm_config.go`
- `pkg/agentcompose/llm_facade.go`
- `pkg/agentcompose/llm_runtime_config.go`

Target packages:

- `pkg/agentcompose/llm`
- `pkg/agentcompose/transport/http/runtimellm`

Scope:

- Extract LLM client, LLM config service, runtime facade token logic, and facade route adapter.
- Preserve OpenAI/Anthropic-compatible behavior and existing environment handling.

Out of scope:

- Model/provider behavior changes.
- API path changes.
- Prompt/runtime guest contract changes.

Suggested validation:

```bash
go test ./pkg/agentcompose -run 'LLM|Facade'
```

#### Task 6: Image Module

Branch:

```text
refactor/image-module
```

Primary files:

- `pkg/agentcompose/image_service.go`
- `pkg/agentcompose/image_auto_backend.go`
- `pkg/agentcompose/image_oci.go`
- `pkg/agentcompose/image_oci_backend.go`
- `pkg/agentcompose/image_ensure.go`

Target package:

- `pkg/agentcompose/image`

Scope:

- Extract image service, image backend abstractions, OCI backend integration, and image ensure logic.
- Keep `pkg/imagecache` unchanged.
- Keep Docker and OCI image behavior unchanged.

Out of scope:

- Image pull policy changes.
- Registry behavior changes.
- Driver image behavior changes.

Suggested validation:

```bash
go test ./pkg/agentcompose -run 'Image'
go test ./pkg/imagecache
```

#### Task 7: Workspace, Config, and Agent Definition Modules

Branch:

```text
refactor/workspace-config-module
```

Primary files:

- `pkg/agentcompose/workspace.go`
- `pkg/agentcompose/workspace_routes.go`
- `pkg/agentcompose/config_store.go`
- `pkg/agentcompose/agent_definition.go`
- `pkg/agentcompose/agent_service.go`

Target packages:

- `pkg/agentcompose/workspace`
- `pkg/agentcompose/configsvc`
- `pkg/agentcompose/agentdef`
- `pkg/agentcompose/transport/http/workspace`

Scope:

- Extract workspace business logic, workspace HTTP route adapter, global env config service, and agent definition service.
- Keep file workspace behavior and config semantics unchanged.

Out of scope:

- File upload/download behavior changes.
- Agent execution behavior changes.
- SQLite physical store split except required adapters.

Suggested validation:

```bash
go test ./pkg/agentcompose -run 'Workspace|Config|AgentDefinition|Agent'
```

### Phase 2: Integration Splits

Phase 2 starts after most Phase 1 branches are merged into the integration branch.

#### Task 8: Transport Split

Branch:

```text
refactor/transport-module
```

Target packages:

- `pkg/agentcompose/app`
- `pkg/agentcompose/transport/connectv1`
- `pkg/agentcompose/transport/connectv2`
- `pkg/agentcompose/transport/http`

Scope:

- Move Connect handlers out of the all-in-one `Service`.
- Move HTTP route registration into transport packages.
- Keep handler behavior, request validation, response shape, and error codes unchanged.

Out of scope:

- Business service changes.
- API design changes.

Suggested validation:

```bash
go test ./pkg/agentcompose ./cmd/agent-compose
```

#### Task 9: SQLite Store Split

Branch:

```text
refactor/sqlite-store-module
```

Target package:

- `pkg/agentcompose/store/sqlite`

Scope:

- Replace all-in-one `ConfigStore` exposure with module-specific repository implementations.
- Keep a shared DB connection object internally.
- Keep schema and migrations compatible.
- Remove temporary repository adapters where safe.

Out of scope:

- Schema redesign.
- Query optimization not required by the split.
- Behavioral changes.

Suggested validation:

```bash
go test ./pkg/agentcompose -run 'Store|Migration|Loader|Project|Event|LLM'
```

### Phase 3: CLI and Cleanup

#### Task 10: CLI and Daemon Split

Branch:

```text
refactor/cli-daemon-module
```

Target packages:

- `internal/daemon`
- `internal/cli`
- `internal/cli/compose`

Scope:

- Move daemon app construction and CLI command implementation out of `cmd/agent-compose/main.go`.
- Keep command names, flags, output behavior, exit codes, and daemon startup behavior unchanged.

Out of scope:

- CLI UX changes.
- Flag changes.
- Output format changes.

Suggested validation:

```bash
go test ./cmd/agent-compose
go test ./internal/...
```

#### Task 11: Compatibility Cleanup

Branch:

```text
refactor/module-split-cleanup
```

Scope:

- Remove temporary compatibility wrappers after all modules compile through new packages.
- Remove empty files and obsolete facade types.
- Update package docs and import paths.

Out of scope:

- New architecture changes.
- Behavior changes.
- Broad formatting churn.

Suggested validation:

```bash
task test
```

## Merge Order

Merge completed branches into `refactor/module-split-integration` in this preferred order:

```text
image
llm
event-webhook
workspace-config
session
loader
project-run
transport
sqlite-store
cli-daemon
cleanup
```

If merge conflicts are substantial, the owner decides whether to rebase a branch onto the latest integration branch or resolve centrally.

## Review Checklist

Before a branch can merge:

- Scope matches the assigned task.
- No unrelated cleanup or behavior changes.
- Existing API, route, env, DB, and runtime contracts remain compatible.
- Temporary wrappers are thin and documented by code structure.
- Tests for the affected module pass.
- `git diff --stat` is consistent with a module move, not broad churn.

## Final Completion Criteria

The module split is complete when:

- `pkg/agentcompose` is no longer a large all-purpose package.
- Connect handlers are separated from core business services.
- `Service` no longer implements all v1 and v2 APIs as a monolithic receiver.
- `ConfigStore` no longer acts as the public all-purpose repository.
- `LoaderManager` is split into clear loader components.
- `cmd/agent-compose/main.go` is reduced to command bootstrap.
- `task test` passes.
- Public behavior remains compatible with the pre-refactor code.
