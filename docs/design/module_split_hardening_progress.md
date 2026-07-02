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
| H2A | Root slimming | Project/store root logic | `refactor/hardening-root-project-store` | `/data/src/github.com/kingfs/agent-compose-hardening-root-project-store` | pending | `assigned` | pending | Keep wrappers and schema compatibility. |
| H2B | Root slimming | Exec/session root logic | `refactor/hardening-root-exec-service` | pending | pending | `not_started` | pending | Keep runtime/session behavior compatible. |
| H2C | Root slimming | Compatibility facade audit | `refactor/hardening-root-compat` | pending | pending | `not_started` | pending | Remove only proven-redundant wrappers. |
| H3A | Large file split | CLI compose | `refactor/hardening-cli-compose` | `/data/src/github.com/kingfs/agent-compose-hardening-cli-compose` | pending | `assigned` | pending | Preserve CLI flags/output/exit behavior. |
| H3B | Large file split | Loader QJS engine | `refactor/hardening-loader-qjs` | `/data/src/github.com/kingfs/agent-compose-hardening-loader-qjs` | pending | `assigned` | pending | Preserve QJS runtime semantics. |
| H3C | Large file split | Loader store | `refactor/hardening-loader-store` | pending | pending | `not_started` | pending | Preserve loader DB behavior. |
| H3D | Large file split | LLM config | `refactor/hardening-llm-config` | `/data/src/github.com/kingfs/agent-compose-hardening-llm-config` | pending | `assigned` | pending | Preserve env/default/provider behavior. |
| H4A | Test relocation | Loader and LLM tests | `refactor/hardening-tests-loader-llm` | pending | pending | `not_started` | pending | Move focused tests near modules. |
| H4B | Test relocation | Project and session tests | `refactor/hardening-tests-project-session` | pending | pending | `not_started` | pending | Keep root integration coverage. |
| H4C | Test relocation | Transport and store tests | `refactor/hardening-tests-transport-store` | pending | pending | `not_started` | pending | Focus on module-owned behavior. |
| H5 | Boundary checks | Dependency boundaries | `refactor/hardening-boundary-checks` | pending | pending | `not_started` | pending | Add repeatable import-boundary check. |

## Planned Merge Order

| Order | Branch | Status | Merge Notes |
| --- | --- | --- | --- |
| 1 | `refactor/hardening-metrics` | `merged` | Baseline metrics merged into integration. |
| 2 | `refactor/hardening-root-project-store` | `assigned` | Merge after metrics baseline. |
| 3 | `refactor/hardening-root-exec-service` | `not_started` | Merge after project/store or in parallel if no conflict. |
| 4 | `refactor/hardening-root-compat` | `not_started` | Merge after root movement branches. |
| 5 | `refactor/hardening-cli-compose` | `assigned` | Independent from root slimming. |
| 6 | `refactor/hardening-loader-qjs` | `assigned` | Independent from CLI. |
| 7 | `refactor/hardening-loader-store` | `not_started` | Coordinate with root/store changes. |
| 8 | `refactor/hardening-llm-config` | `assigned` | Independent from loader. |
| 9 | `refactor/hardening-tests-loader-llm` | `not_started` | After related package splits. |
| 10 | `refactor/hardening-tests-project-session` | `not_started` | After root package slimming. |
| 11 | `refactor/hardening-tests-transport-store` | `not_started` | After store/transport test targets are stable. |
| 12 | `refactor/hardening-boundary-checks` | `not_started` | Last, after imports settle. |

## Worktree Registry

| Worktree Path | Branch | Owner | Status | Cleanup Required |
| --- | --- | --- | --- | --- |
| `/data/src/github.com/kingfs/agent-compose-hardening-metrics` | `refactor/hardening-metrics` | owner | `cleaned` | No |
| `/data/src/github.com/kingfs/agent-compose-hardening-root-project-store` | `refactor/hardening-root-project-store` | pending | `assigned` | Yes |
| `/data/src/github.com/kingfs/agent-compose-hardening-cli-compose` | `refactor/hardening-cli-compose` | pending | `assigned` | Yes |
| `/data/src/github.com/kingfs/agent-compose-hardening-loader-qjs` | `refactor/hardening-loader-qjs` | pending | `assigned` | Yes |
| `/data/src/github.com/kingfs/agent-compose-hardening-llm-config` | `refactor/hardening-llm-config` | pending | `assigned` | Yes |

## Integration Log

| Date | Action | Branch | Result | Notes |
| --- | --- | --- | --- | --- |
| 2026-07-02 | Created hardening plan and progress tracker | `refactor/module-split-integration` | in progress | No production code moved. |
| 2026-07-02 | Created H1 metrics worktree | `refactor/hardening-metrics` | in progress | Worktree `/data/src/github.com/kingfs/agent-compose-hardening-metrics`. |
| 2026-07-02 | Merged H1 metrics baseline | `refactor/hardening-metrics` | passed | `task arch:metrics`; `task test` passed after installing JS runtime dependencies from lockfiles. |
| 2026-07-02 | Cleaned H1 metrics worktree | `refactor/hardening-metrics` | done | Removed worktree after merge. |
| 2026-07-02 | Created H2A/H3A/H3B/H3D worktrees | multiple | in progress | Worktrees created from latest integration for parallel work. |

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
