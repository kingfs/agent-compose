# Module Split Hardening Progress

This document tracks execution of `module_split_hardening_plan.md`.

## Operating Rules

- This tracker is the source of truth for hardening task status.
- Work is assigned at module level only.
- Agents must not split assigned work into unrelated cleanup tasks.
- Agents must not change API contracts, persistence format, runtime contracts, loader script behavior, CLI behavior, or business semantics.
- Every branch must be reviewable by module boundary.
- Every completed branch must include validation notes before merge.

## Status Values

Use only these values:

- `not_started`
- `assigned`
- `in_progress`
- `blocked`
- `ready_for_review`
- `merged`
- `cleaned`

## Branches

| Purpose | Branch | Status | Notes |
| --- | --- | --- | --- |
| Integration | `refactor/module-split-integration` | `in_progress` | Hardening work merges back here. |

## Task Board

| Task | Phase | Module | Branch | Worktree | Assignee | Status | Validation | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- | --- |
| H1 | Metrics baseline | Architecture metrics | `refactor/hardening-metrics` | removed | owner | `cleaned` | `task arch:metrics`; `task test` | Merged into integration; worktree cleaned. |
| H2A | Root slimming | Project/store root logic | `refactor/hardening-root-project-store` | removed | Chandrasekhar (`019f2088-ac94-7002-8c57-c1a532c6e6c7`) | `cleaned` | project/run tests; project/store/sqlite tests; CLI tests | Project SQLite repository moved under `store/sqlite`; root wrappers preserved; worktree cleaned. |
| H2B | Root slimming | Exec/session root logic | `refactor/hardening-root-exec-service` | removed | Huygens (`019f209e-1adc-7061-a801-10471457da23`) | `cleaned` | exec/session focused tests; `go test ./pkg/agentcompose/...` | Pure exec helpers moved to `session`; root wrappers preserved; worktree cleaned. |
| H2C | Root slimming | Compatibility facade audit | `refactor/hardening-root-compat` | removed | Nash (`019f209e-5328-78f1-87cd-14bc1a48a878`) | `cleaned` | `go test ./pkg/agentcompose`; `go test ./cmd/agent-compose ./internal/...` | Audit doc added; only private unreferenced wrappers removed; worktree cleaned. |
| H3A | Large file split | CLI compose | `refactor/hardening-cli-compose` | removed | Kuhn (`019f2088-e166-75b1-b757-d5e8804963fd`) | `cleaned` | `go test ./internal/cli/... ./cmd/agent-compose`; `go test ./cmd/agent-compose -run 'Test.*CLI|Test.*Status|Test.*Host|Test.*Socket'` | Merged into integration; worktree cleaned. |
| H3B | Large file split | Loader QJS engine | `refactor/hardening-loader-qjs` | removed | Hilbert (`019f2089-1427-7ac2-8dcf-61015134082a`) | `cleaned` | `go test ./pkg/agentcompose/loader/qjs ./pkg/agentcompose/loader ./pkg/agentcompose -run 'Loader|QJS|Webhook|Event'` | Merged into integration; worktree cleaned. |
| H3C | Large file split | Loader store | `refactor/hardening-loader-store` | removed | Descartes (`019f2093-86db-72e1-932f-857925b0de6d`) | `cleaned` | loader timestamp/schema tests; `go test ./pkg/agentcompose -run 'TestConfigStore.*Migration|Test.*Loader|Test.*Webhook|Test.*Project'`; `go test ./pkg/agentcompose/loader/...` | Merged into integration; worktree cleaned. |
| H3D | Large file split | LLM config | `refactor/hardening-llm-config` | removed | Bacon (`019f2089-5434-7b72-b1f7-a7be35416fae`) | `cleaned` | `go test ./pkg/agentcompose/llm ./pkg/agentcompose -run 'LLM|Facade|RuntimeConfig|Config'` | Merged into integration; worktree cleaned. |
| H4A | Test relocation | Loader and LLM tests | `refactor/hardening-tests-loader-llm` | removed | Ramanujan (`019f20ad-140c-7d73-933e-a3f6c7e06b51`) | `cleaned` | `go test ./pkg/agentcompose/loader/... ./pkg/agentcompose/llm/...`; loader/LLM root focused tests | Module-level tests added; root integration tests retained; worktree cleaned. |
| H4B | Test relocation | Project and session tests | `refactor/hardening-tests-project-session` | removed | Heisenberg (`019f20ad-43b1-7c20-8901-5755f759e5ba`) | `cleaned` | `go test ./pkg/agentcompose/project ./pkg/agentcompose/session`; project/session root focused tests | Module-level tests added; root integration tests retained; worktree cleaned. |
| H4C | Test relocation | Transport and store tests | `refactor/hardening-tests-transport-store` | removed | Copernicus (`019f20ad-769f-70a3-8bb2-6d6e9274c8ef`) | `cleaned` | `go test ./pkg/agentcompose/store/... ./pkg/agentcompose/transport/...`; store/transport root focused tests | Module-level tests added; root integration tests retained; worktree cleaned. |
| H5 | Boundary checks | Dependency boundaries | `refactor/hardening-boundary-checks` | pending | pending | `not_started` | pending | Add repeatable import-boundary check. |

## Planned Merge Order

| Order | Branch | Status | Merge Notes |
| --- | --- | --- | --- |
| 1 | `refactor/hardening-metrics` | `merged` | Baseline metrics merged into integration. |
| 2 | `refactor/hardening-root-project-store` | `merged` | Project SQLite repository split merged into integration. |
| 3 | `refactor/hardening-root-exec-service` | `merged` | Exec/session helper split merged into integration. |
| 4 | `refactor/hardening-root-compat` | `merged` | Compatibility audit merged into integration. |
| 5 | `refactor/hardening-cli-compose` | `merged` | CLI compose split merged into integration. |
| 6 | `refactor/hardening-loader-qjs` | `merged` | QJS engine split merged into integration. |
| 7 | `refactor/hardening-loader-store` | `merged` | Loader store split merged into integration. |
| 8 | `refactor/hardening-llm-config` | `merged` | LLM config split merged into integration. |
| 9 | `refactor/hardening-tests-loader-llm` | `merged` | Loader/LLM module tests merged into integration. |
| 10 | `refactor/hardening-tests-project-session` | `merged` | Project/session module tests merged into integration. |
| 11 | `refactor/hardening-tests-transport-store` | `merged` | Store/transport module tests merged into integration. |
| 12 | `refactor/hardening-boundary-checks` | `not_started` | Last, after imports settle. |

## Worktree Registry

| Worktree Path | Branch | Owner | Status | Cleanup Required |
| --- | --- | --- | --- | --- |
| `/data/src/github.com/kingfs/agent-compose-hardening-metrics` | `refactor/hardening-metrics` | owner | `cleaned` | No |
| `/data/src/github.com/kingfs/agent-compose-hardening-root-project-store` | `refactor/hardening-root-project-store` | Chandrasekhar (`019f2088-ac94-7002-8c57-c1a532c6e6c7`) | `cleaned` | No |
| `/data/src/github.com/kingfs/agent-compose-hardening-cli-compose` | `refactor/hardening-cli-compose` | Kuhn (`019f2088-e166-75b1-b757-d5e8804963fd`) | `cleaned` | No |
| `/data/src/github.com/kingfs/agent-compose-hardening-loader-qjs` | `refactor/hardening-loader-qjs` | Hilbert (`019f2089-1427-7ac2-8dcf-61015134082a`) | `cleaned` | No |
| `/data/src/github.com/kingfs/agent-compose-hardening-llm-config` | `refactor/hardening-llm-config` | Bacon (`019f2089-5434-7b72-b1f7-a7be35416fae`) | `cleaned` | No |
| `/data/src/github.com/kingfs/agent-compose-hardening-loader-store` | `refactor/hardening-loader-store` | Descartes (`019f2093-86db-72e1-932f-857925b0de6d`) | `cleaned` | No |
| `/data/src/github.com/kingfs/agent-compose-hardening-root-exec-service` | `refactor/hardening-root-exec-service` | Huygens (`019f209e-1adc-7061-a801-10471457da23`) | `cleaned` | No |
| `/data/src/github.com/kingfs/agent-compose-hardening-root-compat` | `refactor/hardening-root-compat` | Nash (`019f209e-5328-78f1-87cd-14bc1a48a878`) | `cleaned` | No |
| `/data/src/github.com/kingfs/agent-compose-hardening-tests-loader-llm` | `refactor/hardening-tests-loader-llm` | Ramanujan (`019f20ad-140c-7d73-933e-a3f6c7e06b51`) | `cleaned` | No |
| `/data/src/github.com/kingfs/agent-compose-hardening-tests-project-session` | `refactor/hardening-tests-project-session` | Heisenberg (`019f20ad-43b1-7c20-8901-5755f759e5ba`) | `cleaned` | No |
| `/data/src/github.com/kingfs/agent-compose-hardening-tests-transport-store` | `refactor/hardening-tests-transport-store` | Copernicus (`019f20ad-769f-70a3-8bb2-6d6e9274c8ef`) | `cleaned` | No |

## Integration Log

| Date | Action | Branch | Result | Notes |
| --- | --- | --- | --- | --- |
| 2026-07-02 | Created hardening plan and progress tracker | `refactor/module-split-integration` | in progress | No production code moved. |
| 2026-07-02 | Created H1 metrics worktree | `refactor/hardening-metrics` | in progress | Worktree `/data/src/github.com/kingfs/agent-compose-hardening-metrics`. |
| 2026-07-02 | Merged H1 metrics baseline | `refactor/hardening-metrics` | passed | `task arch:metrics`; `task test` passed after installing JS runtime dependencies from lockfiles. |
| 2026-07-02 | Cleaned H1 metrics worktree | `refactor/hardening-metrics` | done | Removed worktree after merge. |
| 2026-07-02 | Created H2A/H3A/H3B/H3D worktrees | multiple | in progress | Worktrees created from latest integration for parallel work. |
| 2026-07-02 | Merged and cleaned H2A project SQLite repository split | `refactor/hardening-root-project-store` | passed | Root project store now delegates to `store/sqlite`; compatibility wrappers preserved. |
| 2026-07-02 | Created H2B/H2C root slimming worktrees | root exec-service, root compat | in progress | Worktrees created from latest integration. |
| 2026-07-02 | Merged and cleaned H2C root compatibility audit | `refactor/hardening-root-compat` | passed | Audit document added; private unreferenced wrappers removed. |
| 2026-07-02 | Merged and cleaned H2B exec/session helper split | `refactor/hardening-root-exec-service` | passed | Pure helpers moved to `session`; root execution facade preserved. |
| 2026-07-02 | Created H4 test relocation worktrees | H4A/H4B/H4C | in progress | Worktrees created from latest integration. |
| 2026-07-02 | Merged and cleaned H4B project/session module tests | `refactor/hardening-tests-project-session` | passed | Focused module tests added without deleting root integration tests. |
| 2026-07-02 | Merged and cleaned H4A loader/LLM module tests | `refactor/hardening-tests-loader-llm` | passed | Focused module tests added without deleting root integration tests. |
| 2026-07-02 | Merged and cleaned H4C store/transport module tests | `refactor/hardening-tests-transport-store` | passed | Focused module tests added without deleting root integration tests. |
| 2026-07-02 | Merged and cleaned H3B Loader QJS split | `refactor/hardening-loader-qjs` | passed | Mechanical same-package split; validation passed. |
| 2026-07-02 | Merged and cleaned H3A CLI compose split | `refactor/hardening-cli-compose` | passed | Mechanical same-package split; CLI validation passed. |
| 2026-07-02 | Merged and cleaned H3D LLM config split | `refactor/hardening-llm-config` | passed | Mechanical same-package split; LLM validation passed. |
| 2026-07-02 | Created H3C loader store worktree | `refactor/hardening-loader-store` | in progress | Worktree created from latest integration. |
| 2026-07-02 | Merged and cleaned H3C loader store split | `refactor/hardening-loader-store` | passed | Mechanical same-package split; loader store validation passed. |

## Current Owner Decisions

- Use `refactor/module-split-integration` as the integration branch.
- Use module-sized worktree branches for implementation.
- Start with metrics before code movement.
- Preserve compatibility wrappers unless removal is proven safe.
- Prefer behavior-preserving file splits over deeper redesign.

## Blockers

| Date | Task | Blocker | Owner Decision | Status |
| --- | --- | --- | --- | --- |
| 2026-07-02 | Discovery | codebase-memory MCP transport is closed | Use local Go tooling and shell metrics until MCP recovers. | open |
