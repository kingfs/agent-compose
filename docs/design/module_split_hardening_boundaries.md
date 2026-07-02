# Module Split Hardening Boundary Checks

This document defines the lightweight architecture boundary checks added during
H5 of `module_split_hardening_plan.md`.

The checks intentionally enforce only boundaries that are already expected to
hold after the hardening pass. They do not try to solve every historical
compatibility dependency in one step.

## Checked Boundaries

- `pkg/agentcompose/*` subpackages must not import the root
  `pkg/agentcompose` compatibility package.
- `pkg/agentcompose/store/*` packages must not import `app` or `transport`
  packages.
- Core `pkg/agentcompose/*` packages must not import `app` or `transport`
  packages. The `app` and `transport` packages themselves are excluded from
  this rule.
- `pkg/agentcompose/transport/*` packages must not import concrete
  `store/*` packages directly.

## Not Yet Enforced

Some dependencies still exist for compatibility and are not enforced here:

- proto imports in selected core packages
- `connectrpc.com/connect` imports in packages that still expose compatibility
  error mapping
- Echo imports in active HTTP-adjacent packages such as webhook handling

These can be tightened in a later phase after the compatibility API surface is
reduced further.

## Command

Run:

```bash
task arch:boundaries
```

The task executes:

```bash
./scripts/check-architecture-boundaries.sh
```
