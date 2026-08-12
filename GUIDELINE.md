# Design Guidelines

These principles govern the design and implementation of this project. They are intentionally vendor-neutral and apply regardless of which specific technologies fill each role.

## Core Design Principles

### Minimal and Changeable

- Keep the design minimal and easy to change.
- Prefer explicit contracts between layers before implementation details.
- Keep placeholder packages lightweight until their implementation phase begins.
- Treat prediction and future consumers as downstream of the canonical stores, not as early architectural shortcuts.

### Modular

- Each key component (discovery, collection, normalization, persistence, query) can be replaced independently.
- Preserve modularity so different telemetry sources can evolve independently.
- Keep the core model DRY: define canonical entities, metrics, and graph relations once.

### Decoupled Components

- Separate technology-specific adapters from platform-generic domain logic.
- Adapters own all vendor-specific integration; core packages never import adapter packages.
- Persistence sits behind generic store interfaces; storage backends are swappable without changing domain logic.
- The collection plane is assembled from plans, not from static configuration.

## Vendor Neutrality

- No vendor lock-in: adapter boundaries isolate all technology-specific choices.
- Vendor-specific packages belong exclusively in adapter boundaries, never in core.
- Core domain logic must remain portable across different infrastructure providers and telemetry backends.

## Easy Setup and Integration

- Prefer standard installation methods (Helm, container images, CLI flags).
- Keep prerequisites minimal and well-documented.
- Provide sensible defaults so the system works out of the box.
- New contributors should be able to build, test, and run the system with minimal setup.

## GitOps-Ready

- All configuration must be expressible as YAML or CRD resources.
- Deployment must be automatable through Helm charts with values overrides.
- Runtime behavior must be driven from declarative configuration, not imperative scripts.
- Avoid manual steps that cannot be reproduced in a CI/CD pipeline.
- Configuration changes should be reviewable in version control diffs.

## Testability

- Optimize for testability with small interfaces, deterministic IDs, and validation helpers.
- Keep new behavior covered by focused unit tests.
- Use discovery results to decide which collectors and receivers should be enabled, keeping test scenarios reproducible.

## Dependency Philosophy

- Keep the dependency footprint minimal.
- Do not add framework dependencies before there is a concrete need.
- Use standard library and focused packages over large frameworks.

## Configuration Rules

Never hardcode configuration values. All configurable settings must be externalized through one of:

- **CLI flags** for runtime configuration (with sensible defaults)
- **Environment variables** for deployment-specific settings
- **Taskfile variables** for build/test orchestration (never inline values)
- **Named constants** in code for defaults (never magic strings)

When adding new configuration:

1. Add a CLI flag with a sensible default.
2. Add a corresponding Taskfile variable if used in tests.
3. Update `.env.example` if it is an environment-based setting.
4. Document the option in the relevant `docs/` file.

### Anti-Patterns

```go
// BAD: hardcoded cluster name
adapter := NewDiscoveryAdapter(client, "kind-test")

// GOOD: configurable via flag/parameter
adapter := NewDiscoveryAdapter(client, clusterName)
```

```yaml
# BAD: hardcoded registry name
docker inspect oksg-kind-registry

# GOOD: use variable reference
docker inspect {{.KIND_REGISTRY_NAME}}
```

## Architecture Boundaries

- Discovery results drive which collectors and receivers are enabled.
- Adapters own all vendor-specific integration; core packages never import adapter packages.
- Persistence sits behind generic store interfaces; storage backends are swappable without changing domain logic.
- The collection plane is assembled from plans, not from static configuration.
