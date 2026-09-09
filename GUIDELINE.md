# Design Guidelines

These principles govern the design and implementation of this project. They are intentionally vendor-neutral and apply regardless of which specific technologies fill each role.

## Core Design Principles

### Minimal and Changeable

- Keep the design minimal and easy to change.
- Prefer explicit contracts between layers before implementation details.
- Do not add or retain placeholder packages; every package needs an active,
  reachable ownership contract.
- Treat prediction and future consumers as downstream of the canonical stores, not as early architectural shortcuts.

### Modular

- Each key component (discovery, collection, normalization, persistence, query) can be replaced independently.
- Preserve modularity so different telemetry sources can evolve independently.
- Static source modules own source-specific detection, parsing, conversion, and
  collection requirements through narrow contracts.
- Keep the core model DRY: define canonical entities, metrics, and graph relations once.

### Decoupled Components

- Separate technology-specific adapters from platform-generic domain logic.
- Adapters own technology transport and configuration syntax; static source
  modules own source-specific semantics. Core packages remain source- and
  provider-agnostic and never import adapters.
- Persistence sits behind generic store interfaces; storage backends are swappable without changing domain logic.
- The collection plane is assembled from plans, not from static configuration.

## Vendor Neutrality

- No vendor lock-in: adapter boundaries isolate transport choices, while static
  source modules isolate source semantics.
- Core domain logic remains portable across infrastructure providers and
  telemetry backends.

## Easy Setup and Integration

- Prefer standard installation methods (Helm, container images, CLI flags).
- Keep prerequisites minimal and well-documented.
- Provide sensible defaults so the system works out of the box.
- New contributors should be able to build, test, and run the system with minimal setup.

## GitOps-Ready

- Deployment configuration must be rendered through Helm values and remain
  reviewable in version control.
- Taskfiles and scripts may orchestrate discovery and E2E workflows, but they
  do not replace the deployed runtime configuration.
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

Every operator-controlled setting needs one explicit owner and an external
contract through one of:

- **CLI flags** for runtime configuration (with sensible defaults)
- **Environment variables** for deployment-specific settings
- **Taskfile variables** for build/test orchestration
- **Named constants** in code for defaults (never magic strings)

Fixed policy and invariants belong to their owning compiler, chart, or module
and intentionally have no override unless the product scope requires one.

When adding an operator-controlled configuration input:

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

```bash
# E2E transport contract: both supported cells use this fixed registry.
docker inspect karthos-kind-registry
# Images are published through localhost:5001; no registry override is supported.
```

## Architecture Boundaries

- Discovery results drive which collectors and receivers are enabled.
- Persistence sits behind generic store interfaces; storage backends are swappable without changing domain logic.
- The collection plane is assembled from plans, not from static configuration.
