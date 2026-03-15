# Project Overview

This repository contains the Phase 1 foundation, the Phase 2 discovery/planning slice, and the first Phase 3 implementation slice of a modular observability-driven platform for Kubernetes microservice systems. The goal is to transform heterogeneous runtime observability signals into a canonical, time-aware system graph that combines structure and behavior and can later feed prediction and simulation modules.

The system is organized around stable architectural boundaries between:

- cluster-specific telemetry integration
- collection and routing
- semantic normalization
- topology reconstruction
- persistence
- control and query APIs

Prediction is intentionally deferred as a future consumer of the graph and metrics foundation, not as a concern mixed into the early layers. The current implemented slice now includes discovery-driven OpenTelemetry collector assembly plus the first deployed collection-plane runtime with backend OTLP ingest.

# Tech Stack (Main App)

Keep the main application minimal:

- Go
- Kubernetes Go client for discovery and cluster-side deployment workflows
- `sigs.k8s.io/yaml` for collector config and Helm values rendering
- Prometheus packages only when a concrete Prometheus adapter, parser, or test application needs them
- OTLP packages only when an OTLP adapter or receiver is added
- `github.com/google/uuid` for event identifiers
- `github.com/invopop/jsonschema` for generated schema documentation
- `github.com/stretchr/testify` for tests

Do not add large framework dependencies before there is a concrete need for them.

# Repository Layout

```text
cmd/
  backend/         Main backend binary entrypoint
  controller/      Future controller/operator entrypoint
  discovery-job/   Discovery, planning, and deploy entrypoint
  schema-gen/      Helper for JSON Schema generation

pkg/
  core/
    entities/      Canonical graph-facing entities and stable IDs
    metrics/       Canonical metric definitions, units, and samples
    topology/      Graph nodes, edges, relations, and snapshots
    registry/      Canonical metric registry contracts
    featurestore/  Reserved for future model-ready features
    schema/        Raw telemetry envelopes, validation, schema versions

  adapters/
    kubernetes/          Kubernetes-specific integration boundary
    prometheus/          Prometheus/OpenMetrics adapter boundary
    istio/               Istio integration boundary
    otlp/                OTLP integration boundary
    obi/                 Reserved eBPF/OBI boundary
    autoinstrumentation/ Reserved auto-instrumentation boundary

  collector/
    configgen/      Collector config and Helm values generation
    pipelines/      Collection plane pipeline contracts
    exporters/      Future collector exporters

  discovery/
    detectors/      Detection contracts for observability sources
    profile/        Cluster discovery snapshot contracts

  backend/
    ingest/         Ingestion and semantic processing contracts
    query/          Graph and metrics query contracts
    persistence/    Store interfaces for metrics, topology, metadata

  modeling/
    scenarios/      Reserved scenario definitions
    predictors/     Predictor interfaces only
    simulation/     Reserved simulation boundary

deploy/
  helm/             Helm charts and release packaging
  manifests/        Raw manifest placeholder
  examples/         Example deployment placeholder

test/
  e2e/
    kind/           kind cluster setup scripts and config
    apps/           example microservices for base and Prometheus scenarios
    scenarios/      end-to-end validation scripts and expected outputs
```

# Architecture Principles

- Keep the design minimal and easy to change.
- Prefer explicit contracts between layers before implementation details.
- Use discovery results to decide which OTel receivers should be enabled.
- Separate technology-specific adapters from platform-generic domain logic.
- Preserve modularity so Prometheus, Istio, OTLP, and future inputs can evolve independently.
- Keep the core model DRY by defining canonical entities, metrics, and graph relations once.
- Optimize for testability with small interfaces, deterministic IDs, and validation helpers.
- Treat prediction as a future consumer of the canonical stores, not as an early architectural shortcut.

# Configuration Guidelines

**Never hardcode configuration values.** All configurable settings must be externalized:

- **CLI flags**: Use command-line flags for runtime configuration (e.g., `--cluster-name`, `--namespace`)
- **Environment variables**: Use env vars for deployment-specific settings, with sensible defaults
- **Taskfile variables**: Define all configurable values in the `vars:` section, never inline
- **Constants**: Define named constants in code for default values, never use magic strings

**Examples of what NOT to do:**

```go
// BAD: Hardcoded cluster name
adapter := NewDiscoveryAdapter(client, "kind-test", probe)

// GOOD: Configurable via flag/parameter
adapter := NewDiscoveryAdapter(client, clusterName, probe)
```

```yaml
# BAD: Hardcoded registry name in task
docker inspect oksg-kind-registry

# GOOD: Use variable reference
docker inspect {{.KIND_REGISTRY_NAME}}
```

**When adding new configuration:**

1. Add a CLI flag with a sensible default
2. Add corresponding Taskfile variable if used in e2e tests
3. Update `.env.example` if it's an environment-based setting
4. Document the configuration option in relevant docs

# Build & Test

Primary commands:

- `go mod tidy`
- `task fmt`
- `task test`
- `task build`
- `task generate`
- `task vet`
- `task lint`
- `task test:e2e:setup`
- `task test:e2e:discover`
- `task test:e2e:plan`
- `task test:e2e:apply-otel`
- `task test:e2e:scenario:prometheus`
- `task test:e2e:scenario:fallback`

Guidance:

- Prefer `task ci` before and after meaningful changes.
- Prefer `task test:e2e:scenario:fallback` and `task test:e2e:scenario:prometheus` when changing discovery-driven collection behavior.
- Keep new behavior covered by focused unit tests.
- Use `go generate ./...` or `task generate` to refresh JSON Schema output after schema changes.
- Keep placeholder packages lightweight until their implementation phase begins.

# CI/CD

GitHub Actions runs on PRs and pushes to `main`/`development`:

- **lint**, **format-check**, **vet**, **test**, **build** on all branches
- **security** (govulncheck + gosec) on `main` only

Run all checks locally with `task ci` before pushing.

# Documentation

The root [`README.md`](README.md) and the `docs/` folder contain detailed documentation that must be kept up to date:

| Document | Description |
|----------|-------------|
| `docs/architecture.md` | System layers, component responsibilities, design principles |
| `docs/discovery-flow.md` | Sequence diagrams and pipeline documentation |
| `docs/runtime-flow.md` | Post-deploy telemetry flow from collectors into backend ingest |
| `docs/testing.md` | Unit tests, e2e testing with kind, test scenarios |
| `docs/development.md` | Project structure and contribution guidelines |

**Documentation Update Requirements:**

- Treat the root [`README.md`](README.md) as the top-level implementation and onboarding summary, and keep it aligned with the current project phase and runtime behavior.
- Treat `docs/` as the primary implementation documentation surface. Prefer keeping `docs/` current over adding broad code-level documentation.
- Keep documentation aligned with the actual implementation and current Taskfile commands, not aspirational or outdated flows.
- When the current project phase, implemented runtime surface, or repository entrypoints change, update the root [`README.md`](README.md)
- When adding or modifying adapters, update `docs/architecture.md` with new components
- When changing the discovery or deployment pipeline, update the sequence diagram and operational steps in `docs/discovery-flow.md`
- When changing the deployed runtime telemetry path after Helm apply, update `docs/runtime-flow.md`
- When changing test infrastructure, Helm install flow, or task names, update `docs/testing.md`
- When adding new packages or changing project structure, update `docs/development.md`
- When changing Helm values, ConfigMaps, Secrets, or runtime toggles, update the relevant docs to describe the configuration model
- Keep logging documentation focused on operational behavior in `docs/`:
  structured JSON logs to stdout only
  `kubectl logs` as the debugging path
- Keep diagrams (ASCII and Mermaid) synchronized with actual implementation
- Update example outputs in `docs/discovery-flow.md` when ClusterProfile structure changes
- Remove or rewrite obsolete docs, dead instructions, and stale references in the same change that makes them outdated
