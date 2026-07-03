# Architecture Target Boundaries

This project is moving toward a one-way dependency model:

```text
cmd/agent-compose
  -> internal/app
  -> internal/{agent,loader,run,session,workspace,event,...}
  -> internal/persistence
  -> pkg

internal/transport -> domain/application contracts only
```

`cmd/agent-compose` is bootstrap code. It owns process setup, configuration loading, Echo creation, route registration, and shutdown wiring. Bootstrap may assemble application services, but business packages should not depend on bootstrap packages.

`internal/transport` is the delivery layer. HTTP, proxy, and Connect adapters translate protocol input and output to application calls. Transport must not import `internal/app`; it should depend on narrower application interfaces or route registration contracts. Architecture tests enforce this boundary for all `internal/transport/...` packages.

`internal/app` is the composition and usecase orchestration layer. It wires services, handlers, repositories, background managers, and domain workflows. Echo and Connect handler framework usage belongs here or in transport/bootstrap code, not in domain packages.

`internal/{agent,loader,run,session,workspace,event,...}` are domain and usecase packages. They should remain independent from Echo, Connect handlers, and transport adapters. Generated proto message packages may be used where they are part of the current API model, but generated Connect server packages and handler frameworks should not become domain dependencies.

`internal/persistence` contains storage adapters. Persistence may depend on domain models and lower-level reusable packages, but should not depend on transport adapters, Echo, or Connect handler frameworks.

`pkg` contains reusable packages and must not depend on project `internal` packages.

## Project Capability Target

Project capability migration should converge on this dependency direction:

```text
internal/transport/connect Project handler
  -> internal/app ProjectService facade
  -> internal/project usecase
  -> internal/persistence and pkg
```

During the transition, `internal/app` may continue to host the generated Connect route registration and the ProjectService facade. The target shape is that protocol handling stays in transport/connect adapters, application-facing orchestration stays behind the app facade, and reusable project behavior moves into a future `internal/project` usecase package.

Project Connect adapters belong in `internal/transport/connect`. App routes should only wire those adapters into the service graph and pass them the ProjectService facade or a narrow application interface; request decoding, response encoding, and Connect error mapping should stay in the adapter.

Generated Connect handler packages such as `proto/...connect` are route adapter dependencies. They must not leak into domain or usecase packages. If `internal/project` is introduced, it should not import Echo, `connectrpc.com/connect`, or generated Connect handler packages.

See [project-service-migration.md](project-service-migration.md) for the ProjectService-specific migration target and guardrails.

## Migration Strategy

1. Keep the architecture tests green and expand them when a boundary becomes clean.
2. When removing a known violation, delete its allowlist entry from `internal/architecture/architecture_test.go` in the same change.
3. Move protocol-specific request/response handling toward `internal/transport` and application-specific orchestration toward `internal/app`.
4. Prefer small interfaces at the boundary between transport and app instead of importing broad concrete service graphs.
5. Do not move business code only to satisfy a test; first define the intended owner, then move the dependency in that direction.
