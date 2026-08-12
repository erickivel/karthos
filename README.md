# OKSG — Observability-Driven Kubernetes System Graph

OKSG turns OpenTelemetry-driven cluster discovery and runtime telemetry into a
live, queryable system graph: services, workloads, pods, nodes, and the
relations between them, with metrics attached to nodes and edges.

## What It Does

- discovers Kubernetes resources, Prometheus targets, OTLP-capable workloads, and Istio sidecar mesh
- plans an OTel collection pipeline (agent + gateway + backend) and renders Helm values
- ingests OTLP metrics and traces, normalizes producer-specific names to a semconv-first canonical catalog, and arbitrates one winning producer per metric to prevent double-counting
- reconstructs a topology graph from kubeletstats resource attributes (`runs_on`, `contains`), direct Kubernetes API structure (`exposes`, `routes_to`, `uses`, `mounts`), trace `peer.service` attributes (`calls`), and Istio mesh metrics (`calls` with mesh provenance)
- persists graph, entities, metadata, and metrics behind swappable in-memory or external (PostgreSQL + VictoriaMetrics) backends
- exposes a versioned REST API and a React frontend with three views (Cluster Overview, Topology Graph, Metrics Explorer)

## Components

| Path | Role |
|------|------|
| `cmd/discovery-job` | Discover, plan, and deploy CLI |
| `cmd/backend` | OTLP ingest, normalization, topology builders, REST API, discovery-refresh loop |
| `cmd/schema-gen` | JSON Schema generation helper |
| `pkg/adapters/kubernetes` | Kubernetes API inventory adapter plus live Pod UID/node lifecycle validation |
| `pkg/adapters/otel` | Source planner and Helm values builder |
| `pkg/adapters/otlp` | OTLP gRPC/HTTP server adapter |
| `pkg/adapters/api` | REST API server, handlers, response envelope, pagination |
| `pkg/adapters/storage` | Storage factory; `postgresql/` and `victoriametrics/` external backends |
| `pkg/backend/ingest` | Envelope builder, signal router, normalizer, topology builders, ingestors |
| `pkg/backend/persistence` | Store interfaces (`MetricStore`, `GraphStore`, `MetadataStore`, `RawStore`, `EntityRegistry`) and in-memory implementations |
| `pkg/core/metrics` | Canonical catalog, generic dialect and resolver contracts, OTLP mappings |
| `pkg/sources` | Static source-module inventory with source-owned detection, collector contributions, and normalization |
| `pkg/core/semconv` | OTLP semantic-convention attribute key constants |
| `pkg/core/{entities,topology,schema}` | Canonical entity / topology / envelope types |
| `pkg/discovery/profile` | Capability-only ClusterProfile contract and bounded evidence types |
| `web/` | React frontend (Vite + TypeScript) |
| `deploy/helm/oksg-otel` | Helm chart packaging agent, gateway, backend, frontend, and optional storage subcharts |
| `test/e2e/{apps,scenarios,kind}` | Demo fixtures (`base`, `with-prometheus`, `with-istio`) and scenario scripts |

## Prerequisites

- Go `1.26+`
- [Task](https://taskfile.dev/)
- Docker, kind, kubectl, helm (for e2e)
- `golangci-lint` (optional)

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
```

### E2E with kind

```bash
task test:e2e:setup                       # one control-plane + one worker node

task test:e2e:scenario:prometheus         # Prometheus discovery + servicegraph traces
task test:e2e:scenario:istio              # Istio sidecar mesh scrape
task test:e2e:scenario:fallback           # bare services, kubeletstats only
```

Targeting a remote k3s cluster (or any other supported cell) is a
`CLUSTER_PROFILE` switch in `.env` — see [docs/remote-cluster.md](docs/remote-cluster.md).

Step-by-step (`deploy → discover → plan → apply-otel → verify`) is documented
in [docs/testing.md](docs/testing.md).

After deploying, generate traffic and inspect:

```bash
task test:e2e:traffic
task test:e2e:debug:api       # backend REST API
```

Local port convention:

- `18080` → `gateway-go` traffic generation
- `18081` → backend REST API
- `13133` → backend debug/health

### Frontend topology QA (Playwright)

```bash
task web:e2e:install
task web:dev
kubectl port-forward -n oksg-system deployment/oksg-backend 18081:8080
task web:e2e
task web:e2e:update          # refresh committed screenshot baselines
```

The runner expects the frontend at `http://localhost:5173`; the Vite proxy
forwards `/api/*` to `http://localhost:18081`.

### Cleanup

```bash
task test:e2e:reset           # remove the Helm-owned OTel release and source contract
task test:e2e:reset-apps      # remove demo fixtures
task test:e2e:reset-all       # both
task test:e2e:teardown        # destroy cluster and registry
```

## Documentation

| Document | Description |
|----------|-------------|
| [Architecture](docs/architecture.md) | Layers, components, deployment model, logging policy |
| [Discovery Flow](docs/discovery-flow.md) | Discovery to release-values flow with sequence diagrams |
| [Runtime Flow](docs/runtime-flow.md) | Post-deploy telemetry path through agent, gateway, backend |
| [Metrics Pipeline](docs/metrics-pipeline.md) | Catalog, dialects, source registry, resolver |
| [API Reference](docs/api-reference.md) | REST API routes, query params, response shapes |
| [Frontend Design Spec](docs/frontend-design-spec.md) | Frontend stack, design system, projection modes |
| [Testing](docs/testing.md) | Unit tests, e2e scenarios, traffic generation |
| [Development](docs/development.md) | Project structure, common commands, frontend dev loop |

## License

License is intentionally left undecided at this stage and should be finalized
explicitly later.
