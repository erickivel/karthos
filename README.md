# OKSG — Observability-Driven Kubernetes System Graph

OKSG is a Kubernetes-native observability-to-graph platform that reconstructs a live, explorable system graph from cluster discovery data and OpenTelemetry-centered telemetry collection.

## What It Does

The project is designed to:

- discover services, workloads, pods, nodes, and related cluster resources
- plan and assemble OpenTelemetry Collector configurations from discovered telemetry capabilities
- ingest runtime metrics and interaction signals
- normalize heterogeneous observability data into stable internal contracts
- reconstruct a time-aware topology graph with metrics attached to nodes and edges
- preserve a modular boundary for later prediction, simulation, and reporting features

Phase 1 established the contracts and schemas. Phase 2 adds discovery-driven collection planning and a kind-based test architecture.

## Architecture Overview

The long-term architecture is organized into six layers:

1. Cluster Integration Layer
2. Collection Plane
3. Semantic Processing Layer
4. Persistence Layer
5. Modeling and Prediction Layer
6. Control and API Layer

This repository currently implements the Phase 1 foundation and the first Phase 2 slice:

- canonical entities, metrics, raw telemetry envelopes, and topology contracts
- discovery profile contracts extended with Prometheus target detection and recommended OTel receivers
- Kubernetes discovery and OTel collector planning entrypoints
- manifest rendering and deployment helpers for collector agent/gateway test flows

## Project Status

Current phase: `Phase 2 — Discovery-Driven OTel Collection`

<details>
<summary>Phase 1 — Foundation and Contracts (Completed)</summary>

- Canonical entities with deterministic IDs
- Canonical metric catalog and registry contracts
- Raw telemetry envelope schema and validation
- Topology, graph, and snapshot contracts
- Discovery profile, adapter, collector, query, and persistence interfaces
- JSON Schema generation and CI pipeline

</details>

<details>
<summary>Phase 2 — Discovery-Driven OTel Collection (Current Slice)</summary>

- kind-based test cluster setup with a local registry
- Example microservice variants for `base` and `with-prometheus`
- `k8s-discovery-adapter` for cluster structure and Prometheus endpoint detection
- `otel-source-planner` for collector config generation
- Config rendering plus deploy helpers for OTel agent and gateway manifests
- Scenario coverage for Prometheus discovery and no-telemetry fallback

</details>

Planned roadmap:

1. Foundation and Contracts
2. Cluster Integration and Discovery
3. Collection Plane Implementation
4. Semantic Processing and Canonicalization
5. Topology and Graph Reconstruction
6. Visualizer and Query APIs
7. Future evolution toward prediction and simulation

## Prerequisites

- Go `1.23+`
- [Task](https://taskfile.dev/)

Optional tooling:

- `golangci-lint`
- `kind`
- `kubectl`
- `docker`

## Getting Started

```bash
go mod tidy
task test
task build
task generate
```

## Testing

### Unit Tests

```bash
task test          # run all unit tests
task test:race     # run with race detector
task test:coverage # generate coverage report
```

### E2E Testing with kind

E2E tests require Docker, kind, and kubectl. First, set up the test cluster:

```bash
task test:e2e:setup
```

#### Option 1: Run Full Scenarios (Recommended)

Run a complete scenario that deploys apps, discovers telemetry, generates OTel configs, and verifies:

```bash
# Prometheus discovery scenario (services with /metrics endpoints)
task test:e2e:scenario:prometheus

# No-telemetry fallback scenario (bare services, kubeletstats only)
task test:e2e:scenario:fallback
```

#### Option 2: Step-by-Step Execution

For debugging or custom workflows, run each step individually:

```bash
# 1. Deploy test microservices (choose one variant)
task test:e2e:deploy VARIANT=base            # bare services, no telemetry
task test:e2e:deploy VARIANT=with-prometheus # services with /metrics endpoints

# 2. Run discovery against the cluster
task test:e2e:discover

# 3. Generate OTel collector manifests from discovery
task test:e2e:plan

# 4. Apply the generated collectors to the cluster
task test:e2e:apply-otel

# 5. Verify collectors are running
task test:e2e:verify
```

#### Cleanup

```bash
task test:e2e:reset       # remove OTel collectors only
task test:e2e:reset-apps  # remove demo microservices only
task test:e2e:reset-all   # remove both OTel collectors and demo apps
task test:e2e:teardown    # destroy entire cluster and registry
```

#### Shell Scripts (Alternative)

You can also run the validation scripts directly:

```bash
./test/e2e/scenarios/prometheus-available/run.sh
./test/e2e/scenarios/no-telemetry/run.sh
```

## Documentation

Detailed documentation is available in the [`docs/`](docs/) folder:

- [Architecture](docs/architecture.md) — System layers, components, and design principles
- [Discovery Flow](docs/discovery-flow.md) — Sequence diagrams and pipeline details
- [Testing](docs/testing.md) — Unit tests and e2e testing with kind
- [Development](docs/development.md) — Project structure and contribution guide

## Repository Layout

```text
cmd/
  backend/         Backend API/service entrypoint
  controller/      Future controller/operator entrypoint
  discovery-job/   Future discovery job entrypoint
  schema-gen/      JSON Schema generation helper

pkg/
  adapters/        Technology-specific integration boundaries
  backend/         Ingest, query, and persistence contracts
  collector/       Collection-plane contracts and placeholders
  core/            Canonical entities, metrics, topology, schema, and registry
  discovery/       Detection and cluster profile contracts
  modeling/        Deferred prediction and simulation contracts

deploy/
  helm/            Helm packaging placeholder
  manifests/       Raw manifest placeholder
  examples/        Example deployment placeholder

docs/
  architecture.md  System architecture overview
  discovery-flow.md Pipeline sequence diagrams
  testing.md       Testing guide
  development.md   Development guide

test/
  e2e/
    kind/          kind cluster setup and teardown scripts
    apps/          example microservices and deploy helper
    scenarios/     Prometheus and fallback validation flows
```

## License

License is intentionally left undecided at this stage and should be finalized explicitly later.
