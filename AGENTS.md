# Project Overview

This repository contains the Phase 1 foundation of a modular observability-driven platform for Kubernetes microservice systems. The goal is to transform heterogeneous runtime observability signals into a canonical, time-aware system graph that combines structure and behavior and can later feed prediction and simulation modules.

For the TCC scope, the system is organized around stable architectural boundaries between:

- cluster-specific telemetry integration
- collection and routing
- semantic normalization
- topology reconstruction
- persistence
- control and query APIs

Prediction is intentionally deferred as a future consumer of the graph and metrics foundation, not as a concern mixed into the early layers.

# Tech Stack (Main App)

Keep the main application minimal in Phase 1:

- Go
- Kubernetes Go client in later implementation phases when discovery starts
- Prometheus packages only when a concrete Prometheus adapter or parser is needed
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
```

# Architecture Principles

- Keep the design minimal and easy to change.
- Prefer explicit contracts between layers before implementation details.
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

Guidance:

- Prefer `task test` before and after meaningful changes.
- Keep new behavior covered by focused unit tests.
- Use `go generate ./...` or `task generate` to refresh JSON Schema output after schema changes.
- Keep placeholder packages lightweight until their implementation phase begins.

# CI/CD

GitHub Actions runs on PRs and pushes to `main`/`development`:

- **lint**, **format-check**, **vet**, **test**, **build** on all branches
- **security** (govulncheck + gosec) on `main` only

Run all checks locally with `task ci` before pushing.
