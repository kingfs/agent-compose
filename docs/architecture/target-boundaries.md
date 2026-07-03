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

`internal/transport` is the delivery layer. HTTP, proxy, and Connect adapters translate protocol input and output to application calls. The target boundary is that transport does not import `internal/app`; it should depend on narrower application interfaces or route registration contracts. The current code still has known transport-to-app imports, so architecture tests prevent new imports while that debt is migrated.

`internal/app` is the composition and usecase orchestration layer. It wires services, handlers, repositories, background managers, and domain workflows. Echo and Connect handler framework usage belongs here or in transport/bootstrap code, not in domain packages.

`internal/{agent,loader,run,session,workspace,event,...}` are domain and usecase packages. They should remain independent from Echo, Connect handlers, and transport adapters. Generated proto message packages may be used where they are part of the current API model, but generated Connect server packages and handler frameworks should not become domain dependencies.

`internal/persistence` contains storage adapters. Persistence may depend on domain models and lower-level reusable packages, but should not depend on transport adapters, Echo, or Connect handler frameworks.

`pkg` contains reusable packages and must not depend on project `internal` packages.

## Migration Strategy

1. Keep the architecture tests green and expand them when a boundary becomes clean.
2. When removing a known violation, delete its allowlist entry from `internal/architecture/architecture_test.go` in the same change.
3. Move protocol-specific request/response handling toward `internal/transport` and application-specific orchestration toward `internal/app`.
4. Prefer small interfaces at the boundary between transport and app instead of importing broad concrete service graphs.
5. Do not move business code only to satisfy a test; first define the intended owner, then move the dependency in that direction.
