# Karthos - Kubernetes Data Foundation

Karthos turns OpenTelemetry-driven cluster discovery and runtime telemetry into a
live, queryable system graph: services, workloads, pods, nodes, and the
relations between them, with metrics attached to nodes and edges.

## What It Does

- discovers Kubernetes resources, Prometheus targets, OTLP-capable workloads, and Istio sidecar mesh
- plans an OTel collection pipeline (agent + gateway + backend) and renders Helm values
- ingests OTLP metrics and traces, normalizes producer-specific names to a semconv-first canonical catalog, and arbitrates one winning producer per metric to prevent double-counting
- reconstructs a topology graph from direct Kubernetes API structure (`contains`, `exposes`, `routes_to`, `uses`, `mounts`), validated metric resource attributes (`runs_on`), and traces or source-classified calls metrics such as Istio mesh telemetry (`calls`)
- persists graph, entities, metadata, and metrics behind swappable in-memory or external (PostgreSQL + VictoriaMetrics) backends
- exposes a versioned REST API and a React frontend with three views (Cluster Overview, Topology Graph, Metrics Explorer)

## Components

| Path | Role |
|------|------|
| `cmd/discovery-job` | Discover, plan, and deploy CLI |
| `cmd/backend` | OTLP ingest, normalization, topology builders, REST API, discovery-refresh loop |
| `cmd/schema-gen` | JSON Schema generation helper |
| `pkg/adapters/kubernetes` | Kubernetes API inventory adapter plus live Pod UID/node lifecycle validation |
| `pkg/adapters/otel` | OpenTelemetry adapter: translates generic collection plans into collector configuration |
| `pkg/adapters/otlp` | OTLP gRPC ingress server adapter |
| `pkg/adapters/api` | REST API server, handlers, response envelope, pagination |
| `pkg/adapters/storage` | Storage factory; `postgresql/` and `victoriametrics/` external backends |
| `pkg/backend/ingest` | Envelope builder, signal router, normalizer, topology builders, ingestors |
| `pkg/backend/persistence` | Store interfaces (`MetricStore`, `GraphStore`, `MetadataStore`, `EntityRegistry`) and in-memory implementations |
| `pkg/core/metrics` | Canonical catalog, provider-neutral instrument semantics, resolver contracts, samples, aggregation, and units |
| `pkg/collector` | Generic collection-plan composition, provider-neutral requirements, Helm values compilation, and deployment |
| `pkg/sources` | Static source-module inventory with source-owned detection, raw metric parsing/conversion, semantic collection requirements, and normalization |
| `pkg/core/semconv` | OTLP semantic-convention attribute key constants |
| `pkg/core/{entities,topology,schema}` | Canonical entity / topology / envelope types |
| `pkg/discovery/profile` | Capability-only ClusterProfile contract and bounded evidence types |
| `web/` | React frontend (Vite + TypeScript) |
| `deploy/helm/karthos` | Helm chart packaging agent, gateway, backend, frontend, and optional storage subcharts |
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
task test:e2e:scenario:istio              # resets owned resources before the mesh check; retains the new environment
task test:e2e:scenario:fallback           # strict direct-source baseline
```

Targeting a remote k3s cluster (or any other supported cell) is a
`CLUSTER_PROFILE` switch in `.env`; see [docs/remote-cluster.md](docs/remote-cluster.md).

`task deploy` validates and deploys the current working tree, including
uncommitted backend and frontend source. Complete scenarios own fixture setup,
deployment, verification, and restoration; see [docs/testing.md](docs/testing.md).

After deploying, generate traffic and inspect:

```bash
task test:e2e:traffic
task test:e2e:debug:api       # backend REST API
```

Local port convention:

- `18080` → `gateway-go` traffic generation
- `18081` → backend REST API
- `13133` → backend health checks

### Cleanup

```bash
task undeploy                 # remove only the platform release and owned PostgreSQL PVC
task test:e2e:reset           # remove platform, fixtures, data, and E2E-owned Istio
task test:e2e:teardown        # destroy cluster and registry
```

## Documentation

| Document | Description |
|----------|-------------|
| [Architecture](docs/architecture.md) | Layers, components, deployment model, logging policy |
| [Discovery Flow](docs/discovery-flow.md) | Discovery to release-values flow with sequence diagrams |
| [Runtime Flow](docs/runtime-flow.md) | Post-deploy telemetry path through agent, gateway, backend |
| [Metrics Pipeline](docs/metrics-pipeline.md) | Catalog, source parsers, source registry, resolver |
| [API Reference](docs/api-reference.md) | REST API routes, query params, response shapes |
| [OpenAPI](docs/openapi.yaml) | Machine-readable REST API description |
| [Frontend Design Spec](docs/frontend-design-spec.md) | Frontend stack, design system, projection modes |
| [Testing](docs/testing.md) | Unit tests, e2e scenarios, traffic generation |
| [Development](docs/development.md) | Project structure, common commands, frontend dev loop |

## License

Licensed under the [Apache License, Version 2.0](LICENSE). See [NOTICE](NOTICE).

The revision evaluated in the thesis is `4c2861b539fcd24d6a458dc0fd545fbdda5470d5`
(tag `tcc-2026`); the evaluation lives in
[karthos-tcc-evaluation](https://github.com/erickivel/karthos-tcc-evaluation).
