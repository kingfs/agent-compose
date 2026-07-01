# Module Split Refactor Progress

This document is the execution tracker for `module_split_refactor_plan.md`.

The owner must update this file whenever a task is assigned, a worktree is created, a branch is merged, a blocker appears, or a worktree is cleaned.

## Operating Rules

- This tracker is the source of truth for task status.
- Work is assigned at module level only.
- Agents must not split assigned work into unrelated cleanup tasks.
- Agents must not change API contracts, persistence format, runtime contracts, loader script behavior, or business semantics.
- Every branch must be reviewable by module boundary.
- Every completed branch must include validation notes before merge.

## Status Values

Use only these status values:

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
| Baseline governance | `refactor/module-split-base` | `ready_for_review` | Governance docs committed; module branches are created from this baseline. |
| Integration | `refactor/module-split-integration` | `in_progress` | Created from baseline; module branches will merge here in planned order. |

## Task Board

| Task | Module | Branch | Worktree | Assignee | Status | Validation | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | Baseline and governance | `refactor/module-split-base` | main worktree | owner | `ready_for_review` | Docs only | Plan and tracker committed; integration branch created. |
| 1 | Session | `refactor/session-module` | `../agent-compose-refactor-session` | pending launch | `assigned` | `go test ./pkg/agentcompose ./pkg/driver` | Wave 2 task; preserve session API and runtime behavior. |
| 2 | Loader | `refactor/loader-module` | `../agent-compose-refactor-loader` | pending launch | `assigned` | `go test ./pkg/agentcompose -run 'Loader|Webhook|Event'` | Wave 2 task; preserve loader DB schema and script API. |
| 3 | Project and Run | `refactor/project-run-module` | `../agent-compose-refactor-project-run` | pending launch | `assigned` | `go test ./pkg/agentcompose -run 'Project|Run|Compose'` | Wave 2 task; preserve ApplyProject and run semantics. |
| 4 | Event and Webhook | `refactor/event-webhook-module` | removed | Hypatia (`019f1e08-16de-7653-a575-1b736b059058`) | `cleaned` | `go test ./pkg/agentcompose -run 'Event|Webhook|Topic'`; integration combo test passed | Merged into integration as `23577d6`; worktree cleaned. |
| 5 | LLM | `refactor/llm-module` | removed | Faraday (`019f1e08-16b2-74e1-b1b7-fa3e37fca0ba`) | `cleaned` | `go test ./pkg/agentcompose -run 'LLM|Facade'`; integration combo test passed | Merged into integration as `8085896`; worktree cleaned. |
| 6 | Image | `refactor/image-module` | removed | Newton (`019f1e08-1689-78e2-b0aa-67f0d03450d2`) | `cleaned` | `go test ./pkg/agentcompose -run 'Image'`; `go test ./pkg/imagecache`; integration combo test passed | Merged into integration as `bb23ed3`; worktree cleaned. |
| 7 | Workspace, Config, Agent Definition | `refactor/workspace-config-module` | `../agent-compose-refactor-workspace-config` | pending launch | `assigned` | `go test ./pkg/agentcompose -run 'Workspace|Config|AgentDefinition|Agent'` | Wave 2 task; preserve config and file workspace behavior. |
| 8 | Transport | `refactor/transport-module` | `../agent-compose-refactor-transport` | unassigned | `not_started` | `go test ./pkg/agentcompose ./cmd/agent-compose` | Start after Phase 1 merges. |
| 9 | SQLite Store | `refactor/sqlite-store-module` | `../agent-compose-refactor-sqlite-store` | unassigned | `not_started` | `go test ./pkg/agentcompose -run 'Store|Migration|Loader|Project|Event|LLM'` | Start after Phase 1 merges. |
| 10 | CLI and Daemon | `refactor/cli-daemon-module` | `../agent-compose-refactor-cli-daemon` | unassigned | `not_started` | `go test ./cmd/agent-compose` and `go test ./internal/...` | Start after app/transport structure settles. |
| 11 | Compatibility Cleanup | `refactor/module-split-cleanup` | `../agent-compose-refactor-cleanup` | owner | `not_started` | `task test` | Remove wrappers after all splits merge. |

## Planned Merge Order

| Order | Branch | Status | Merge Notes |
| --- | --- | --- | --- |
| 1 | `refactor/image-module` | `merged` | Merged into integration as `bb23ed3`. |
| 2 | `refactor/llm-module` | `merged` | Merged into integration as `8085896`. |
| 3 | `refactor/event-webhook-module` | `merged` | Merged into integration as `23577d6`. |
| 4 | `refactor/workspace-config-module` | `assigned` | Wave 2; watch interaction with session/project. |
| 5 | `refactor/session-module` | `assigned` | Wave 2; larger dependency surface. |
| 6 | `refactor/loader-module` | `assigned` | Wave 2; depends on session/event ports. |
| 7 | `refactor/project-run-module` | `assigned` | Wave 2; depends on loader/image ports. |
| 8 | `refactor/transport-module` | `not_started` | Should follow module service extraction. |
| 9 | `refactor/sqlite-store-module` | `not_started` | Should follow repository interface stabilization. |
| 10 | `refactor/cli-daemon-module` | `not_started` | Should follow app/transport stabilization. |
| 11 | `refactor/module-split-cleanup` | `not_started` | Final wrapper cleanup. |

## Worktree Registry

| Worktree Path | Branch | Owner | Status | Cleanup Required |
| --- | --- | --- | --- | --- |
| `/data/src/github.com/kingfs/agent-compose-refactor-image` | `refactor/image-module` | Newton (`019f1e08-1689-78e2-b0aa-67f0d03450d2`) | `cleaned` | No |
| `/data/src/github.com/kingfs/agent-compose-refactor-llm` | `refactor/llm-module` | Faraday (`019f1e08-16b2-74e1-b1b7-fa3e37fca0ba`) | `cleaned` | No |
| `/data/src/github.com/kingfs/agent-compose-refactor-event-webhook` | `refactor/event-webhook-module` | Hypatia (`019f1e08-16de-7653-a575-1b736b059058`) | `cleaned` | No |
| `/data/src/github.com/kingfs/agent-compose-refactor-session` | `refactor/session-module` | pending launch | `assigned` | Yes |
| `/data/src/github.com/kingfs/agent-compose-refactor-loader` | `refactor/loader-module` | pending launch | `assigned` | Yes |
| `/data/src/github.com/kingfs/agent-compose-refactor-project-run` | `refactor/project-run-module` | pending launch | `assigned` | Yes |
| `/data/src/github.com/kingfs/agent-compose-refactor-workspace-config` | `refactor/workspace-config-module` | pending launch | `assigned` | Yes |

## Integration Log

| Date | Action | Branch | Result | Notes |
| --- | --- | --- | --- | --- |
| 2026-07-01 | Created plan and progress tracker | `refactor/module-split-base` | in progress | No production code moved. |
| 2026-07-01 | Created baseline branch | `refactor/module-split-base` | done | Existing `.gitignore` change remains uncommitted and outside refactor scope. |
| 2026-07-01 | Committed governance docs | `refactor/module-split-base` | done | Commit `a2ea0b1`. |
| 2026-07-01 | Created integration branch | `refactor/module-split-integration` | done | Branch created from baseline. |
| 2026-07-01 | Created Wave 1 worktrees | image, llm, event-webhook | done | Ready for parallel module work. |
| 2026-07-01 | Launched Wave 1 agents | image, llm, event-webhook | in progress | Newton=image, Faraday=LLM, Hypatia=event/webhook. |
| 2026-07-01 | Reviewed and committed Wave 1 worker branches | image, llm, event-webhook | done | Owner reproduced module tests before merge. |
| 2026-07-01 | Merged Wave 1 branches into integration | image, llm, event-webhook | done | Merge commits `bb23ed3`, `8085896`, `23577d6`. |
| 2026-07-01 | Ran integration Wave 1 tests | `refactor/module-split-integration` | passed | `go test ./pkg/agentcompose -run 'Image|LLM|Facade|Event|Webhook|Topic'`; `go test ./pkg/agentcompose/image ./pkg/agentcompose/llm ./pkg/agentcompose/event ./pkg/agentcompose/webhook ./pkg/imagecache`. |
| 2026-07-01 | Cleaned Wave 1 worker worktrees | image, llm, event-webhook | done | Removed the three worker worktrees after confirming clean status. |
| 2026-07-01 | Prepared Wave 2 assignments | workspace-config, session, loader, project-run | in progress | Owner will create worktrees from latest integration and launch workers. |

## Current Owner Decisions

- Use module-sized tasks only.
- Preserve compatibility through wrappers during migration.
- Do not start transport or SQLite physical store split until Phase 1 module boundaries are mostly merged.
- Prefer merging lower-dependency modules first: image, LLM, event/webhook.
- Avoid API, schema, runtime, and CLI behavior changes during this refactor.
- Wave 1 assignments are image, LLM, and event/webhook.
- Wave 2 assignments are workspace/config/agent definition, session, loader, and project/run.

## Blockers

| Date | Task | Blocker | Owner Decision | Status |
| --- | --- | --- | --- | --- |
| 2026-07-01 | Wave 2 | Next module worktrees not created yet | Start workspace-config, session, loader, and project/run after owner launches Wave 2. | open |
