# ProjectService Migration Target

Project capabilities are being separated from the current broad app service without changing behavior. The target is a narrow, one-way stack:

```text
Connect API
  -> internal/transport/connect Project handler
  -> internal/app ProjectService facade
  -> internal/project usecase
  -> persistence adapters and reusable pkg code
```

## Target Responsibilities

`internal/transport/connect` owns Project protocol adaptation. It should receive generated Connect requests, translate request and response shapes, call an application boundary, and return protocol errors in Connect form. It may depend on generated Connect handler packages and `connectrpc.com/connect`.

Project Connect adapters should live in `internal/transport/connect`. App route code may wire those adapters into the service graph, but should not become the owner of Project request decoding, response encoding, or Connect error mapping.

`internal/app` owns the ProjectService facade during the migration. The facade is the compatibility layer that keeps route wiring and callers stable while the implementation moves out of the large service. It should expose application-oriented operations and delegate project-specific behavior instead of accumulating more project logic. App routes should remain wiring only: construct or register the Project adapter and pass it the facade or a narrow application interface.

The current facade may temporarily implement generated Project Connect handler methods while operation groups are in flight. That allowance is a migration bridge, not the target owner model. Once an operation group's request decoding, response construction, and Connect error mapping move into `internal/transport/connect`, the matching app facade methods should drop generated Connect request/response signatures and expose application-level inputs and outputs instead.

`internal/project` is the future usecase package for Project behavior. It should contain project validation, apply/get/list/remove orchestration, and policy that is independent from HTTP, Echo, and generated Connect server packages. It may depend on project model types, persistence interfaces, and reusable lower-level packages.

The initial `internal/project` package is intentionally only a foundation layer. It defines transport-agnostic error classification and lightweight result structures, such as validation issues and apply changes, so later usecase code can return stable internal shapes before any Connect or HTTP mapping happens.

Project record and list-result data shapes that used to be tied to persistence have been lifted to `internal/projecttypes`. New project usecase boundaries should accept and return those shared model types, or interfaces expressed in terms of those types, instead of importing concrete persistence adapter packages.

Persistence adapters remain under `internal/persistence`. They should implement storage concerns for project state without depending on transport handlers.

## Migration Guardrails

Generated Connect handler packages are delivery-layer dependencies. They are allowed in app route wiring, transport adapters, bootstrap wiring, and the health route adapter, but they should not be imported by domain or usecase packages.

Current-stage migration keeps `internal/app` as the ProjectService facade. That facade may import `internal/project` foundation types such as `project.ApplyResult`, `project.Change`, `project.ValidationIssue`, and `project.Error` while it continues to preserve the existing Connect API surface. This direction is intentional: app code adapts current handlers and response shapes, while project foundation code stays independent of delivery concerns.

When `internal/project` is added, keep it transport agnostic. It must not import:

- `internal/app`
- `internal/app/...`
- `internal/transport/...`
- `internal/persistence/...` adapter packages
- `connectrpc.com/connect`
- `github.com/labstack/echo/v4`
- generated Connect handler packages under `proto/...connect`

Generated proto message packages may remain part of the current API model where necessary. The stricter boundary is specifically against Connect handler/server packages and handler frameworks.

Foundation types should prefer internal Go structures over proto messages unless the usecase boundary would otherwise duplicate a stable domain model. Protocol message conversion belongs at the facade or transport edge during the migration.

Project record and list shapes live in `internal/projecttypes`, so usecase code can depend on stable project data shapes without depending on a concrete persistence implementation. `internal/project` must not import persistence adapters such as `internal/persistence/sqlite`; adapters satisfy project-owned interfaces from outside the usecase package.

The foundation package must not remain idle during the facade migration. Architecture tests require `internal/app` to import and use `internal/project` foundation types while Project behavior is still being extracted out of the broad app service. If that import disappears, either the migration has completed and the guard should be replaced with a stricter usecase-boundary check, or the facade has regressed to proto/app-local shapes.

## Incremental Steps

1. Keep the existing `internal/app` ProjectService behavior as the public application facade.
2. Move Get/List/Remove protocol mapping into `internal/transport/connect`: generated request extraction, protobuf response assembly, and Connect code mapping belong there.
3. Change the app facade for Get/List/Remove to expose application-level methods that accept project query inputs and return project query results or `internal/project` errors, without generated Connect handler ownership.
4. Keep app route changes limited to wiring the `internal/transport/connect` Project adapter to the facade or a narrow application interface.
5. Extract remaining project usecases behind narrow interfaces into `internal/project`.
6. After Get/List/Remove, migrate Apply and Validate using the same split: transport owns protocol mapping, app exposes application-level methods, and `internal/project` owns operation policy.
7. Delete transitional app logic only after equivalent usecase coverage exists.

When extracting apply behavior, return `project.ApplyResult` and `project.Error` from the new usecase first, then translate those values in the app facade or Connect adapter. This keeps persistence and orchestration tests independent from Connect status codes while preserving the current API response shape until the facade can be thinned further.

## Next-Stage Convergence Plan

The next migration stage should move large behavior blocks in dependency order, while keeping the current Connect API green at every step.

First, complete the Query/Remove facade thinning already started by `internal/project.QueryUsecase`. Get/List/Remove should have their protocol mapping in `internal/transport/connect`, including project reference extraction, response protobuf construction, and `project.ErrorKind` to Connect code mapping. The app facade should expose application-level methods for those operations, such as query/remove request structs and result structs, and should not remain the permanent owner of generated Connect handler signatures for those methods.

Second, sink Apply orchestration further into `internal/project`. The usecase should own normalization-adjacent policy, validation aggregation, dry-run planning, persistence sequencing, and reconciliation result assembly behind transport-neutral inputs and outputs. The app facade may continue to translate from generated proto requests and to generated proto responses while this lands, but it should stop owning new Apply business decisions.

Third, migrate Apply protocol mapping to `internal/transport/connect` once the application-level Apply boundary is stable. The Connect adapter should map generated Apply requests to app-level inputs, map `project.ApplyResult` and project errors back to protobuf responses and Connect errors, and leave the app facade without generated Connect handler ownership for Apply.

Fourth, migrate Validate by the same pattern. Validation policy and issue aggregation should live behind `internal/project`, the app facade should expose a validation method in application terms, and `internal/transport/connect` should own generated request/response and Connect error mapping.

Fifth, introduce store interfaces at the project boundary before moving persistence-heavy code. The interfaces should describe the operations the usecases need, such as project lookup, revision save/load, agent and scheduler listing, managed resource updates, and removal state changes. `internal/persistence` and the current app config store can adapt to those interfaces; `internal/project` should not import concrete app service types or Connect handler packages.

After `internal/projecttypes` adoption, keep the architecture guard strict: `internal/project` must not import `internal/persistence/sqlite` or any other persistence adapter directly; adapters should satisfy project-owned interfaces from outside the usecase package.

Sixth, keep thinning the facade. Once a usecase owns an operation, the app method should be limited to application orchestration and usecase invocation; generated request extraction, Connect error classification, and protobuf response mapping should sit in `internal/transport/connect`. Route registration can still wire the facade during the transition, but new Project protocol adaptation belongs in `internal/transport/connect`.

Finally, tighten the architecture tests after the large blocks have landed. The current guards require a live `internal/project` foundation, prevent delivery and persistence dependencies from entering it, and prevent app Project files from reviving migration artifacts or app-local private error classifications. After Apply and Query/Remove are owned by `internal/project`, replace the temporary allowance for app Project logic with stricter checks that app is only wiring and mapping, and that `internal/transport/connect` depends on a narrow application interface rather than the broad app package.
