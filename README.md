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

Phase 1 established the contracts and schemas. Phase 2 added discovery-driven collection planning, Phase 3 implements the end-to-end OpenTelemetry collection plane with a real backend OTLP ingest runtime, Phase 4 adds topology reconstruction from runtime telemetry with debug endpoints for graph and metrics inspection, Phase 5 adds durable persistence with PostgreSQL and VictoriaMetrics behind swappable storage interfaces, and Phase 6 exposes a REST API layer with enriched graph queries, entity management, metric series, and system status endpoints.

## Architecture Overview

The long-term architecture is organized into six layers:

1. Cluster Integration Layer
2. Collection Plane
3. Semantic Processing Layer
4. Persistence Layer
5. Modeling and Prediction Layer
6. Control and API Layer

This repository currently implements Phases 1 through 6:

- canonical entities, metrics, raw telemetry envelopes, and topology contracts
- discovery profile contracts extended with Prometheus target detection and recommended OTel receivers
- Kubernetes discovery and OTel collector planning entrypoints
- Helm values generation and Helm-managed deployment for agent, gateway, and backend
- OTLP gRPC/HTTP ingest in the backend with raw-envelope conversion and signal routing
- semantic normalization with entity resolution and canonical metric mapping
- structural topology extraction from kubeletstats resource attributes (`runs_on`, `contains` edges)
- K8s structural topology from discovery profile (`exposes`, `routes_to`, `uses`, `mounts` edges)
- OTLP trace-based topology extraction (`calls` edges from `peer.service` span attributes)
- `/debug/graph` endpoint plus `/debug/metrics` for in-memory metric inspection; use `/api/v1/metrics` when external VictoriaMetrics storage is enabled
- durable persistence with PostgreSQL (graph, entities, metadata, raw envelopes) and VictoriaMetrics (metrics) behind generic store interfaces
- storage factory with swappable `memory` and `external` backends
- traffic generation tasks for exercising the demo app and observing graph construction
- REST API with enriched graph nodes, entity management, metric series, and system status
- cursor-based pagination on all list endpoints
- configurable staleness threshold for system health status

## Project Status

Current phase: `Phase 6 — Query APIs and Visualization`

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
<summary>Phase 2 — Discovery-Driven OTel Collection (Completed Slice)</summary>

- kind-based test cluster setup with a local registry
- Example microservice variants for `base` and `with-prometheus`
- `k8s-discovery-adapter` for cluster structure and Prometheus endpoint detection
- `otel-source-planner` for collector config generation
- Typed Helm values generation for the collector release
- Scenario coverage for Prometheus discovery and no-telemetry fallback

</details>

<details>
<summary>Phase 3 — OpenTelemetry Collection Plane Implementation (Completed)</summary>

- Helm-managed deployment of agent, gateway, and backend
- Backend OTLP ingest over gRPC and HTTP
- Raw-envelope conversion and signal routing in `pkg/backend/ingest`
- End-to-end telemetry verification through structured backend logs
- Runtime flow documentation for the deployed collection plane

</details>

<details>
<summary>Phase 4 — Topology Reconstruction (Completed)</summary>

- Semantic normalization with `OTLPEntityResolver` and `MetricNormalizer`
- `MetricTopologyBuilder` derives `runs_on` and `contains` edges from kubeletstats resource attributes
- `TraceTopologyBuilder` derives `calls` edges from OTLP trace spans with `peer.service`
- `MultiIngestor` fans envelopes through normalization and topology extraction in parallel
- `InMemoryGraphStore` accumulates nodes and edges across telemetry batches
- `/debug/graph` and `/debug/metrics` HTTP endpoints for runtime inspection (`/debug/metrics` is in-memory-store oriented)
- OTLP trace instrumentation in `gateway-go` demo app
- Traffic generation and debug tasks (`task test:e2e:traffic`, `task test:e2e:debug:graph`)

</details>

<details>
<summary>Phase 5 — Persistence (Completed)</summary>

- Generic store interfaces (`MetricStore`, `GraphStore`, `MetadataStore`, `RawStore`, `EntityRegistry`) in `pkg/backend/persistence/`
- In-memory implementations for all interfaces (tests and default mode)
- Storage factory with `memory` (default) and `external` backends
- PostgreSQL adapter for graph topology, entity registry, metadata, and raw envelopes via `pgx/v5`
- VictoriaMetrics adapter for time-series metrics via Prometheus HTTP API
- Backend CLI flags: `--storage-backend`, `--postgres-dsn`, `--victoriametrics-url`, `--raw-store-enabled`
- Helm chart storage configuration with DSN secret and VM URL

</details>

<details>
<summary>Phase 6 — Query APIs and Visualization (Current)</summary>

- REST API server with enriched graph, entity, metric, and status endpoints
- Cursor-based pagination on all list endpoints (default page_size=100, max=1000)
- Graph summary with kind_counts, relation_counts, available_filters, snapshot_age
- Entity enrichment from registry with OTLP label resolution (name, namespace, cluster)
- Node detail with enriched inbound/outbound edges and related entity summaries
- `/metrics/series` endpoint grouping samples by metric name with catalog metadata
- Normalized backend status (nominal/degraded/disconnected) with configurable staleness threshold
- Backend CLI flags: `--api-listen-address`, `--status-stale-seconds`

</details>

Planned roadmap:

1. Foundation and Contracts (done)
2. Cluster Integration and Discovery (done)
3. Collection Plane Implementation (done)
4. Semantic Processing and Topology Reconstruction (done)
5. Persistence (done)
6. Query APIs and Visualization (current)
7. Future evolution toward prediction and simulation

## Prerequisites

- Go `1.26+`
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

The default kind topology includes one control-plane node plus one worker
node so the topology projection QA scenarios can assert real cross-node
placement.

#### Option 1: Run Full Scenarios (Recommended)

Run a complete scenario that deploys apps, discovers telemetry, generates OTel configs, and verifies:

```bash
# Prometheus discovery scenario (services with /metrics endpoints)
task test:e2e:scenario:prometheus

# Istio mesh telemetry scenario (sidecar mesh scrape)
task test:e2e:scenario:istio

# No-telemetry fallback scenario (bare services, kubeletstats only)
task test:e2e:scenario:fallback
```

#### Option 2: Step-by-Step Execution

For debugging or custom workflows, run each step individually:

```bash
# 1. Deploy test microservices (choose one variant)
task test:e2e:deploy VARIANT=base            # bare services, no telemetry
task test:e2e:deploy VARIANT=with-prometheus # services with /metrics endpoints
task test:e2e:deploy VARIANT=with-istio      # services observed through Istio sidecars

# 2. Run discovery against the cluster
task test:e2e:discover

# 3. Generate OTel Helm values from discovery
task test:e2e:plan

# 4. Apply the generated Helm release to the cluster
task test:e2e:apply-otel

# 5. Verify agent, gateway, and backend are running
task test:e2e:verify
```

#### Traffic Generation and Debug

After deploying, generate traffic and inspect the resulting topology graph:

```bash
# Send requests to gateway-go to generate observable traffic
task test:e2e:traffic

# Verify the backend REST API on its dedicated local port
task test:e2e:debug:api

# Inspect the topology graph built from runtime telemetry
task test:e2e:debug:graph

# Inspect metric samples stored in the backend
task test:e2e:debug:metrics
```

Local port convention used by the e2e helpers:

- `18080` -> `gateway-go` traffic generation
- `18081` -> backend REST API
- `13133` -> backend debug/health endpoints

#### Frontend Topology QA with Playwright

Projection-mode coverage runs against the live frontend and a port-forwarded
backend:

```bash
# install frontend deps and Playwright support in Docker
task web:e2e:install

# start the frontend
task web:dev

# expose the backend API
kubectl port-forward -n oksg-system deployment/oksg-backend 18081:8080

# run the topology suite
task web:e2e

# refresh committed screenshots when intended UI changes land
task web:e2e:update
```

The Playwright runner expects the frontend at `http://localhost:5173` and uses
the Vite proxy to reach the backend API at `http://localhost:18081`.

#### Cleanup

```bash
task test:e2e:reset       # remove OTel collectors and the applied discovery-profile ConfigMap
task test:e2e:reset-apps  # remove demo fixtures from oksg-demo and oksg-sparse
task test:e2e:reset-all   # remove both OTel collectors and all demo fixtures
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
- [Runtime Flow](docs/runtime-flow.md) — Post-deploy telemetry flow through agent, gateway, and backend ingest
- [Testing](docs/testing.md) — Unit tests and e2e testing with kind
- [Development](docs/development.md) — Project structure and contribution guide

## Repository Layout

```text
cmd/
  backend/         Backend OTLP ingest runtime entrypoint
  controller/      Future controller/operator entrypoint
  discovery-job/   Discovery, planning, and deploy entrypoint
  schema-gen/      JSON Schema generation helper

pkg/
  adapters/        Technology-specific integration boundaries
    kubernetes/    Kubernetes discovery adapter
    otel/          OTel source planner
    otlp/          OTLP gRPC/HTTP server adapter
    storage/       Storage factory and config (memory or external)
    storage/postgresql/      PostgreSQL stores (pgx/v5)
    storage/victoriametrics/ VictoriaMetrics MetricStore (HTTP API)
  backend/         Ingest, query, and persistence contracts
    ingest/        Envelope building, routing, normalization, topology
    persistence/   Store interfaces and in-memory implementations
    query/         Graph and metrics query contracts
  collector/       Collection-plane contracts and config generation
  core/            Canonical entities, metrics, topology, schema, and registry
  discovery/       Detection and cluster profile contracts
  modeling/        Deferred prediction and simulation contracts

deploy/
  helm/            Helm chart and release packaging
  manifests/       Raw manifest placeholder
  examples/        Example deployment placeholder

docs/
  architecture.md  System architecture overview
  discovery-flow.md Pipeline sequence diagrams
  runtime-flow.md  Post-deploy runtime telemetry flow
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
