# Project Overview

This repository contains the Phase 1 foundation plus the first Phase 2 implementation slice of a modular observability-driven platform for Kubernetes microservice systems. The goal is to transform heterogeneous runtime observability signals into a canonical, time-aware system graph that combines structure and behavior and can later feed prediction and simulation modules.

For the TCC scope, the system is organized around stable architectural boundaries between:

- cluster-specific telemetry integration
- collection and routing
- semantic normalization
- topology reconstruction
- persistence
- control and query APIs

Prediction is intentionally deferred as a future consumer of the graph and metrics foundation, not as a concern mixed into the early layers. Phase 2 currently focuses on discovery-driven OpenTelemetry collector assembly for Prometheus-or-fallback scenarios.

# Tech Stack (Main App)

Keep the main application minimal:

- Go
- Kubernetes Go client for discovery and cluster-side deployment workflows
- `sigs.k8s.io/yaml` for collector manifest rendering
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
  discovery-job/   Future batch discovery entrypoint
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
    configgen/      Future collector config generation
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
  helm/             Helm packaging placeholder
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
- `task test:e2e:scenario:prometheus`
- `task test:e2e:scenario:fallback`

Guidance:

- Prefer `task test` before and after meaningful changes.
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

The `docs/` folder contains detailed documentation that must be kept up to date:

| Document | Description |
|----------|-------------|
| `docs/architecture.md` | System layers, component responsibilities, design principles |
| `docs/discovery-flow.md` | Sequence diagrams and pipeline documentation |
| `docs/testing.md` | Unit tests, e2e testing with kind, test scenarios |
| `docs/development.md` | Project structure and contribution guidelines |

**Documentation Update Requirements:**

- When adding or modifying adapters, update `docs/architecture.md` with new components
- When changing the discovery pipeline, update the sequence diagram in `docs/discovery-flow.md`
- When adding new test scenarios or changing test infrastructure, update `docs/testing.md`
- When adding new packages or changing project structure, update `docs/development.md`
- Keep diagrams (ASCII and Mermaid) synchronized with actual implementation
- Update example outputs in `docs/discovery-flow.md` when ClusterProfile structure changes
