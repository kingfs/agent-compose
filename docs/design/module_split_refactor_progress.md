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
| Baseline governance | `refactor/module-split-base` | `in_progress` | Created from `main`; governance docs are the Phase 0 payload. |
| Integration | `refactor/module-split-integration` | `not_started` | Merge module branches here in planned order. |

## Task Board

| Task | Module | Branch | Worktree | Assignee | Status | Validation | Notes |
| --- | --- | --- | --- | --- | --- | --- | --- |
| 0 | Baseline and governance | `refactor/module-split-base` | main worktree | owner | `in_progress` | Docs only | Plan and tracker created; pending commit and integration branch creation. |
| 1 | Session | `refactor/session-module` | `../agent-compose-refactor-session` | unassigned | `not_started` | `go test ./pkg/agentcompose ./pkg/driver` | Preserve session API and runtime behavior. |
| 2 | Loader | `refactor/loader-module` | `../agent-compose-refactor-loader` | unassigned | `not_started` | `go test ./pkg/agentcompose -run 'Loader|Webhook|Event'` | Preserve loader DB schema and script API. |
| 3 | Project and Run | `refactor/project-run-module` | `../agent-compose-refactor-project-run` | unassigned | `not_started` | `go test ./pkg/agentcompose -run 'Project|Run|Compose'` | Preserve ApplyProject and run semantics. |
| 4 | Event and Webhook | `refactor/event-webhook-module` | `../agent-compose-refactor-event-webhook` | unassigned | `not_started` | `go test ./pkg/agentcompose -run 'Event|Webhook|Topic'` | Preserve event dispatch and webhook behavior. |
| 5 | LLM | `refactor/llm-module` | `../agent-compose-refactor-llm` | unassigned | `not_started` | `go test ./pkg/agentcompose -run 'LLM|Facade'` | Preserve LLM provider and facade behavior. |
| 6 | Image | `refactor/image-module` | `../agent-compose-refactor-image` | unassigned | `not_started` | `go test ./pkg/agentcompose -run 'Image'` and `go test ./pkg/imagecache` | Preserve Docker and OCI image behavior. |
| 7 | Workspace, Config, Agent Definition | `refactor/workspace-config-module` | `../agent-compose-refactor-workspace-config` | unassigned | `not_started` | `go test ./pkg/agentcompose -run 'Workspace|Config|AgentDefinition|Agent'` | Preserve config and file workspace behavior. |
| 8 | Transport | `refactor/transport-module` | `../agent-compose-refactor-transport` | unassigned | `not_started` | `go test ./pkg/agentcompose ./cmd/agent-compose` | Start after Phase 1 merges. |
| 9 | SQLite Store | `refactor/sqlite-store-module` | `../agent-compose-refactor-sqlite-store` | unassigned | `not_started` | `go test ./pkg/agentcompose -run 'Store|Migration|Loader|Project|Event|LLM'` | Start after Phase 1 merges. |
| 10 | CLI and Daemon | `refactor/cli-daemon-module` | `../agent-compose-refactor-cli-daemon` | unassigned | `not_started` | `go test ./cmd/agent-compose` and `go test ./internal/...` | Start after app/transport structure settles. |
| 11 | Compatibility Cleanup | `refactor/module-split-cleanup` | `../agent-compose-refactor-cleanup` | owner | `not_started` | `task test` | Remove wrappers after all splits merge. |

## Planned Merge Order

| Order | Branch | Status | Merge Notes |
| --- | --- | --- | --- |
| 1 | `refactor/image-module` | `not_started` | Low dependency surface. |
| 2 | `refactor/llm-module` | `not_started` | Keep facade route compatibility. |
| 3 | `refactor/event-webhook-module` | `not_started` | Needed by loader but can expose stable ports. |
| 4 | `refactor/workspace-config-module` | `not_started` | Watch interaction with session/project. |
| 5 | `refactor/session-module` | `not_started` | Larger dependency surface. |
| 6 | `refactor/loader-module` | `not_started` | Depends on session/event ports. |
| 7 | `refactor/project-run-module` | `not_started` | Depends on loader/image ports. |
| 8 | `refactor/transport-module` | `not_started` | Should follow module service extraction. |
| 9 | `refactor/sqlite-store-module` | `not_started` | Should follow repository interface stabilization. |
| 10 | `refactor/cli-daemon-module` | `not_started` | Should follow app/transport stabilization. |
| 11 | `refactor/module-split-cleanup` | `not_started` | Final wrapper cleanup. |

## Worktree Registry

| Worktree Path | Branch | Owner | Status | Cleanup Required |
| --- | --- | --- | --- | --- |
| TBD | TBD | TBD | `not_started` | No |

## Integration Log

| Date | Action | Branch | Result | Notes |
| --- | --- | --- | --- | --- |
| 2026-07-01 | Created plan and progress tracker | `refactor/module-split-base` | in progress | No production code moved. |
| 2026-07-01 | Created baseline branch | `refactor/module-split-base` | done | Existing `.gitignore` change remains uncommitted and outside refactor scope. |

## Current Owner Decisions

- Use module-sized tasks only.
- Preserve compatibility through wrappers during migration.
- Do not start transport or SQLite physical store split until Phase 1 module boundaries are mostly merged.
- Prefer merging lower-dependency modules first: image, LLM, event/webhook.
- Avoid API, schema, runtime, and CLI behavior changes during this refactor.

## Blockers

| Date | Task | Blocker | Owner Decision | Status |
| --- | --- | --- | --- | --- |
| 2026-07-01 | All | Integration branch and module worktrees not created yet | Commit governance docs on baseline branch, then create integration branch and assign Wave 1 worktrees. | open |
