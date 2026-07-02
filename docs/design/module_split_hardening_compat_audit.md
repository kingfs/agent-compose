# H2C Root Compatibility Facade Audit

This audit tracks compatibility wrappers that remain in `pkg/agentcompose` after
the module split. The goal is to keep behavior stable while making future
slimming decisions traceable.

Audit method:

- Code discovery: codebase-memory graph for root package symbols and local
  `rg` for current worktree references.
- Removal rule: only private root wrappers/helpers with no current references
  and no public compatibility role were removed.
- Non-removal rule: exported functions/types, `ConfigStore` methods, `Service`
  methods, Connect/HTTP entrypoints, and type aliases are retained by default.

## Removed In This Pass

| Root symbol | Previous target | Evidence | Safety note |
| --- | --- | --- | --- |
| `matchesTimeRange` | `session.MatchesTimeRange` | `rg` found only the root definition in the current worktree. | Private helper; root callers now use `session.MatchesListOptions`, whose subpackage implementation still uses the real helper. |
| `toProtoAgentWorkFiles` | `agentdef.ToProtoAgentWorkFiles` | `rg` found only the root definition. | Private proto helper; the exported subpackage helper remains available. |
| `agentWorkspaceSummary` | `agentdef.AgentWorkspaceSummary` | `rg` found only the root definition. | Private helper; no root caller. |
| `toProtoAgentCurrentRunSummary` | `agentdef.ToProtoAgentCurrentRunSummary` | `rg` found only the root definition. | Private proto helper; no root caller. |
| `formatProtoTime` | `agentdef.FormatProtoTime` | `rg` found only the root definition. | Private helper; no root caller. |
| `loaderDefaultCronTimezone` | duplicate of loader schedule constant | `rg` found only the root definition. | Private duplicate; root facade calls exported loader schedule functions. |
| `loaderCronSpec` | duplicate of loader schedule shape | `rg` found only the root definition. | Private duplicate; root facade calls exported loader schedule functions. |

## Retained Compatibility Surface

| Area | Root facade symbols | Current callers | Keep reason | Later migration condition |
| --- | --- | --- | --- | --- |
| Session model aliases | `Session`, `SessionSummary`, `SessionListOptions`, `SessionWorkspace`, `NotebookCell`, `AgentRun`, `ExecResult`, `VMState`, `ProxyState`, `ExecSpec`, related constants | Broad root package code, tests, service and driver adapters | Public root package API and shared model vocabulary. | Only remove after external consumers and generated/HTTP service code import `pkg/agentcompose/session` directly. |
| Session private helpers | `sessionEnvMap`, `restoreSessionTransientFields`, list/proto helpers | `service.go`, `agent_service.go`, `run_session.go`, `loader_session_runner.go`, `session_rpc_bridge.go`, tests | Root services still operate on root aliases and need stable helper names during split. | Migrate root services to call subpackage helpers directly, then remove private shims with `rg` proof. |
| Agent definition aliases/helpers | `AgentDefinition`, list/result/validation/run summary aliases, `normalizeAgentDefinition`, `agentDefinitionTags`, `sessionHasAgentTag`, `toProtoAgentDefinition`, `toProtoEnvItems` | `ConfigStore`, `AgentService`, `Service`, `LoaderSessionRunner`, project validation | Public aliases plus active private bridge to `agentdef`. | After `ConfigStore`/services move to `agentdef` types or import the subpackage helpers directly. |
| Loader model/schedule facades | loader constants and type aliases; `normalizeLoader*`, `loaderTrigger*`, `defaultLoader*`, cron schedule wrappers | Loader store, manager, event dispatcher, webhook queue, project service, tests | Root loader services and store still depend on root names; aliases preserve public API. | Move loader store/manager/service out of root or switch call sites to `pkg/agentcompose/loader`. |
| Loader engine facade | `LoaderHost`, `LoaderEngine`, `QJSLoaderEngine`, `NewLoaderEngine`, `loaderEngineMaxExecutionTime` | DI setup, loader manager/tests | Maintains root construction and test seams while QJS engine lives under `loader/qjs`. | Replace root DI registration/callers with `qjs.NewLoaderEngine` and subpackage interfaces. |
| Loader bus/dispatcher/run/session facades | `NewLoaderBus`, `NewLoaderEventDispatcher`, `NewLoaderRunExecutor`, `NewLoaderSessionRunner` and related methods | `Setup`, event dispatcher, scheduler/manager tests | Active root orchestration components; not pure dead wrappers. | Split loader orchestration out of root or introduce subpackage-owned DI graph. |
| Project store/run facades | project record aliases, stable ID constructors, `RunCoordinator`, `ProjectRun*Request`, transition/source helpers | `ConfigStore`, project service, run service, tests | Public constructors and active adapters over project/run subpackages. | Migrate store and run services to subpackage repositories/coordinators; preserve API through a release boundary first. |
| Project session facade | `ProjectSessionRelationFilter`, `ProjectSessionStatus`, `ListProjectSessionStatuses`, `ConfigStore` project-session methods | project service, reconciliation/tests | Public-ish query helper and `ConfigStore` methods are in the strict keep list. | Keep until callers use `project` repository directly; do not remove `ConfigStore` methods without an API deprecation. |
| Runtime/session driver facades | `Driver`, `SessionDriver`, `RuntimeProvider`, `BoxRuntime`, `NewDriver`, `NewRuntimeProvider`, driver/runtime adapters | `Setup`, service lifecycle, loader/session tests | Runtime construction remains rooted in the service graph and adapts legacy driver package types. | Move DI and adapters into `runtime` once services no longer depend on root aliases. |
| Image facades | image request/result/backend aliases, backend constructors, image `Service` methods, `ensureProjectAgentImages`, `ensureDriverImage`, `imageBackendErrorIsNotFound`, `ociMetadataToProtoImage` | image Connect v2 methods, project/run image ensure, tests | Exported aliases/constructors and `Service` methods are public compatibility/entrypoints; private ensure wrappers are active. | Migrate image APIs to `pkg/agentcompose/image` service directly; keep root `Service` methods until Connect registration changes. |
| Workspace facades | private workspace config aliases and helpers around file/git workspace preparation | session lifecycle, service create/load paths, run preparation, workspace routes, tests | Active root service workflow and tests use root model aliases. | Move session/run workspace preparation to `workspace` package with subpackage model imports. |
| Topic event/webhook facades | topic/event constants and aliases; validation/normalization/hash helpers; webhook route/queue wrappers | topic event store, webhook handler/queue, loader dispatch tests | Public event model aliases and active root persistence/service methods. | Migrate event store and webhook HTTP layer to `event`/`webhook` packages fully, then remove private root shims. |
| LLM facades | LLM config/model/token aliases, root config store methods, runtime facade HTTP helpers | service, runtime config, LLM facade tests, HTTP routes | LLM config persistence and HTTP facade remain root-owned entrypoints. | Split config store/HTTP facade ownership first; exported API remains until consumers migrate. |
| Connect/HTTP registration facades | `Setup`, `Register`, `StartBackground`, route registration helpers, proxy/workspace/runtime LLM route helpers | `cmd/agent-compose`, app routes, tests | Explicit entrypoints; strict boundary says keep. | Only revisit after a route registration API replacement exists. |

## Follow-up Rules

- Before deleting another wrapper, capture both graph trace and current-worktree
  `rg` evidence. Prefer current worktree when graph data is stale.
- Public aliases and root constructors are compatibility API unless a separate
  migration/deprecation task says otherwise.
- Private wrappers used only by tests are still compatibility/test seams; remove
  them only when tests can be moved to the target subpackage without changing
  behavior.
