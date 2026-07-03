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

`internal/project` is the future usecase package for Project behavior. It should contain project validation, apply/get/list/remove orchestration, and policy that is independent from HTTP, Echo, and generated Connect server packages. It may depend on project model types, persistence interfaces, and reusable lower-level packages.

The initial `internal/project` package is intentionally only a foundation layer. It defines transport-agnostic error classification and lightweight result structures, such as validation issues and apply changes, so later usecase code can return stable internal shapes before any Connect or HTTP mapping happens.

Persistence adapters remain under `internal/persistence`. They should implement storage concerns for project state without depending on transport handlers.

## Migration Guardrails

Generated Connect handler packages are delivery-layer dependencies. They are allowed in app route wiring, transport adapters, bootstrap wiring, and the health route adapter, but they should not be imported by domain or usecase packages.

When `internal/project` is added, keep it transport agnostic. It must not import:

- `internal/app`
- `connectrpc.com/connect`
- `github.com/labstack/echo/v4`
- generated Connect handler packages under `proto/...connect`

Generated proto message packages may remain part of the current API model where necessary. The stricter boundary is specifically against Connect handler/server packages and handler frameworks.

Foundation types should prefer internal Go structures over proto messages unless the usecase boundary would otherwise duplicate a stable domain model. Protocol message conversion belongs at the facade or transport edge during the migration.

## Incremental Steps

1. Keep the existing `internal/app` ProjectService behavior as the public application facade.
2. Move protocol-only request/response conversion toward `internal/transport/connect`.
3. Keep app route changes limited to wiring the `internal/transport/connect` Project adapter to the facade.
4. Extract project usecases behind narrow interfaces into `internal/project`.
5. Point the app facade at the extracted usecase while preserving the Connect API surface.
6. Delete transitional app logic only after equivalent usecase coverage exists.

When extracting apply behavior, return `project.ApplyResult` and `project.Error` from the new usecase first, then translate those values in the app facade or Connect adapter. This keeps persistence and orchestration tests independent from Connect status codes while preserving the current API response shape until the facade can be thinned further.
