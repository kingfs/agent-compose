# Module Split Hardening Plan

## Purpose

This document controls the post module-split hardening work for agent-compose.

The previous refactor established module boundaries while preserving behavior. This hardening pass reduces remaining root-package weight, splits the largest multi-responsibility files, moves tests closer to the new modules, and adds lightweight architecture checks. It must preserve existing API contracts, runtime behavior, persistence formats, loader semantics, and CLI behavior.

Implementation agents must follow this plan exactly. Do not turn assigned work into broad cleanup, style rewrites, renaming campaigns, or behavior changes.

## Non-Negotiable Constraints

- Keep Connect APIs, HTTP routes, proto files, SQLite schema, and session filesystem layout compatible.
- Keep runtime guest contracts compatible.
- Keep loader script API and execution semantics compatible.
- Keep environment variable names and defaults compatible.
- Preserve CLI command behavior, flags, output semantics, and exit behavior.
- Prefer wrappers and adapters when needed to keep existing call sites stable.
- Add or move tests only to prove moved behavior remains the same.
- Do not mix feature work with hardening.

## Owner Role

The owner maintains this plan and the progress tracker, creates worktree branches from `refactor/module-split-integration`, assigns module-sized work to agents, reviews branch scope, merges completed work back to the integration branch, runs validation, cleans merged worktrees, and updates progress after material state changes.

Implementation agents must work only inside the assigned task boundary, preserve behavior, avoid opportunistic rewrites, report blockers early, and keep changes reviewable at package/module level.

## Baseline Observations

Initial size signals after the module split:

```text
pkg/agentcompose                         34543 LOC / 95 Go files
pkg/agentcompose/loader                   2860 LOC / 8 Go files
pkg/agentcompose/llm                      2438 LOC / 4 Go files
internal/cli/compose                      2054 LOC / 1 Go file
pkg/agentcompose/loader/qjs               1600 LOC / 1 Go file
pkg/agentcompose/event                    1357 LOC / 3 Go files
pkg/agentcompose/image                    1100 LOC / 5 Go files
pkg/agentcompose/webhook                  1065 LOC / 2 Go files
```

Largest production files to govern:

```text
internal/cli/compose/compose.go           2054
pkg/agentcompose/project_service.go       1706
pkg/agentcompose/loader/qjs/engine.go     1600
pkg/agentcompose/service.go               1503
pkg/agentcompose/loader/store.go          1465
pkg/agentcompose/llm/config.go            1227
pkg/agentcompose/loader_manager.go        1217
pkg/agentcompose/exec.go                  1001
pkg/agentcompose/event/store.go            999
pkg/agentcompose/project_store.go          920
```

These are signals for review, not automatic rewrite targets. Generated code, table-driven tests, compatibility facades, and dense mapping code may remain larger when splitting would reduce clarity.

## Engineering Rules

Use this dependency direction:

```text
transport -> app/service module -> module-owned ports -> infrastructure implementation
```

Rules:

- Core modules must not import Connect, proto transport handlers, or Echo.
- Store implementations belong under `pkg/agentcompose/store/*`.
- Module tests should live with the module when they validate module-owned behavior.
- Root `pkg/agentcompose` may keep compatibility wrappers and integration tests during migration.
- `cmd/agent-compose` should stay a thin entrypoint.
- `internal/cli` and `internal/daemon` should own process-level CLI and daemon wiring.

Split files only when there is a clear responsibility boundary. Acceptable split axes include command wiring vs flag parsing vs output rendering vs API calls, model/defaults vs env parsing vs validation vs client construction, repository interface vs SQL implementation vs row mapping, scheduler vs executor vs session adapter vs event dispatch, and facade wrapper vs module-owned business logic.

Metrics are used to focus review, not to create mechanical churn.

Recommended warning thresholds:

- Production file above 600 lines: review for responsibility split.
- Production file above 1000 lines: split unless there is a documented reason not to.
- Package above 3000 lines: review exported API, cohesion, and dependency direction.
- Root `pkg/agentcompose` should trend downward over this hardening pass.

## Phase Plan

### Phase 1: Metrics and Architecture Baseline

Branch:

```text
refactor/hardening-metrics
```

Scope:

- Add repeatable architecture metrics script or task.
- Produce a checked-in baseline report for package LOC, largest files, test placement, and Go package counts.
- Do not move production logic.

Exit criteria:

- Metrics can be regenerated locally.
- Baseline report is committed.
- No behavior changes.

Suggested validation:

```bash
task test
```

### Phase 2: Root Package Slimming

Branches:

```text
refactor/hardening-root-project-store
refactor/hardening-root-exec-service
refactor/hardening-root-compat
```

Scope:

- Move clearly owned project/store logic from root into `project` or `store/sqlite` while keeping wrappers.
- Move clearly owned exec/session service logic into module packages while keeping wrappers.
- Reduce root package to compatibility facade, setup, adapters, and integration tests.

Out of scope:

- CLI restructuring.
- Loader QJS internal split.
- Behavior or schema changes.

Suggested validation:

```bash
go test ./pkg/agentcompose ./pkg/agentcompose/project ./pkg/agentcompose/store/sqlite ./pkg/agentcompose/session
```

### Phase 3: Large File Responsibility Split

Branches:

```text
refactor/hardening-cli-compose
refactor/hardening-loader-qjs
refactor/hardening-loader-store
refactor/hardening-llm-config
```

Scope:

- Split `internal/cli/compose/compose.go` by CLI responsibility.
- Split `pkg/agentcompose/loader/qjs/engine.go` by QJS bootstrap, bindings, execution, and result mapping.
- Split `pkg/agentcompose/loader/store.go` by repository responsibility.
- Split `pkg/agentcompose/llm/config.go` by env parsing, defaults, validation, and runtime config.

Out of scope:

- CLI UX changes.
- Loader execution semantics changes.
- LLM provider behavior changes.

Suggested validation:

```bash
go test ./internal/... ./cmd/agent-compose
go test ./pkg/agentcompose/loader ./pkg/agentcompose/loader/qjs ./pkg/agentcompose/llm ./pkg/agentcompose
```

### Phase 4: Test Relocation and Coverage Proximity

Branches:

```text
refactor/hardening-tests-loader-llm
refactor/hardening-tests-project-session
refactor/hardening-tests-transport-store
```

Scope:

- Move or add focused module-level tests for behavior that now belongs to new packages.
- Keep root integration tests for cross-module workflows.
- Avoid rewriting assertions except where import/package placement requires it.

Out of scope:

- Broad coverage campaigns.
- New feature test cases unrelated to moved behavior.

Exit criteria:

- Key new packages no longer rely only on root package tests.
- `task test` passes.

### Phase 5: Dependency Boundary Checks

Branch:

```text
refactor/hardening-boundary-checks
```

Scope:

- Add lightweight dependency-boundary verification.
- Prefer a local script or task that fails on forbidden imports.
- Document allowed and forbidden dependency directions.

Out of scope:

- Introducing large new frameworks solely for architecture checks.
- Refactoring code only to satisfy speculative future layering.

Exit criteria:

- Boundary check is repeatable.
- Check is integrated into an appropriate task or documented quality gate.
- `task test` passes.

## Planned Merge Order

1. Metrics and architecture baseline.
2. Root package slimming branches.
3. Large file responsibility split branches.
4. Test relocation branches.
5. Dependency boundary checks.
6. Final verification and progress closure.

## Final Verification

The owner must run:

```bash
task test
```

The owner should also run build/lint if practical:

```bash
task build
task lint
```

If build or lint cannot be run, the blocker must be recorded in the progress tracker.
