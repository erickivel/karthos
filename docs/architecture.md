# Architecture Overview

OKSG turns Kubernetes discovery and OpenTelemetry-driven runtime telemetry
into a queryable system graph. Control flow:

```text
Kubernetes API -> ClusterProfile -> Resolver -> SourcePlanner -> Helm Values -> Helm Release
                                                                        |
                                                                        v
                                          Backend (OTLP ingest, normalization, topology, REST API)
                                                                        |
                                                                        v
                                                              React frontend (web/)
```

## Components

| Path | Role |
|------|------|
| `pkg/adapters/kubernetes` | Discovery adapter plus live Pod lifecycle validation — Prometheus targets, OTLP-capable workloads, Istio sidecar mesh, and UID/node List/Watch state |
| `pkg/adapters/otel` | Source planner; Helm values builder; `transform/oksg-source` processor |
| `pkg/adapters/otlp` | OTLP gRPC + HTTP ingest server |
| `pkg/adapters/api` | Versioned REST API server (handlers, response envelope, pagination) |
| `pkg/adapters/storage` | Storage factory; `postgresql/` and `victoriametrics/` external implementations |
| `pkg/collector/{configgen,deployer}` | Collector config → typed Helm values → `helm lint/template/upgrade-install` |
| `pkg/backend/ingest` | Envelope build, signal routing, normalization, topology builders, ingestors |
| `pkg/backend/persistence` | Store interfaces and in-memory implementations |
| `pkg/core/metrics` | Catalog, dialect, source registry, resolver — see [metrics-pipeline.md](metrics-pipeline.md) |
| `pkg/core/semconv` | Shared OTLP semantic-convention attribute key constants |
| `pkg/core/{entities,topology,schema}` | Canonical entity / topology / envelope types |
| `pkg/discovery/{profile,detectors}` | `ClusterProfile`, `ObservabilitySource`, detector contract |
| `cmd/discovery-job` | `discover` / `plan` / `deploy` CLI |
| `cmd/backend` | OTLP ingest, normalization, REST API, structural-topology refresh loop |
| `web/` | React frontend (Vite + TypeScript) — Cluster Overview, Topology Graph, Metrics Explorer |
| `deploy/helm/oksg-otel` | Helm chart packaging agent, gateway, backend, frontend, optional storage subcharts |

`pkg/adapters/{autoinstrumentation,istio,obi,prometheus}` are reserved
adapter boundaries (`doc.go` only) for future integrations.

## Collection Plane

The source planner emits collector intent that the values builder turns into
typed Helm values. Receivers and connectors:

- kubeletstats as component ID `kubelet_stats` (always; pod, container, volume metrics)
- hostmetrics as component ID `host_metrics` (always; per-node CPU, memory, disk, network, load, paging, filesystem, and connection data). Agent pods use the host network by default so interface counters describe the node; opting out removes only the network scraper rather than publishing collector-pod traffic as node truth.
- `k8scluster` (always on the gateway; workload/controller counts, pod phases, node conditions, resource quotas, HPA replica counts)
- `prometheus` on the singleton gateway for application Service-DNS targets (scrape jobs stamp `oksg_source=prometheus.app`)
- `mesh-istio` Prometheus job (only when discovery finds a complete sidecar mesh; each DaemonSet agent scrapes only `istio-proxy:15020/stats/prometheus` Pods on its own node, rewrites Istio labels into servicegraph `client`/`server` edge attributes, and stamps `oksg_source=mesh.istio`)
- `otlp` receivers (when discovery finds workloads with `oksg.io/otlp-traces: "true"` or ports `4317`/`4318`)
- spanmetrics as component ID `span_metrics` (gateway, when traces are present; cluster-wide single instance to avoid double-counting; v0.156 instance-ID dimension disabled for compatibility)
- servicegraph as component ID `service_graph` (gateway, when traces are present and the mesh does not own the POC service-graph coverage; it is the sole selected source for a partial mesh)

Collector-owned agent and gateway pipelines run a `transform/oksg-source`
processor that stamps the Prometheus-safe `oksg_source` datapoint attribute from
the receiver-determined metric-name namespace. The backend accepts either
`oksg_source` or dotted `oksg.source` from producers. External OTLP metrics use isolated `metrics/otlp` pipelines without
inference and must arrive with a registered source. Together with scrape-time relabel stamps
(`oksg_source=prometheus.app` / `mesh.istio`) this gives the backend a
reliable signal for source arbitration.

## Semantic Normalization

The backend ingest pipeline:

| Component | File | Role |
|-----------|------|------|
| `OTLPEntityResolver` | `pkg/backend/ingest/entity_resolver.go` | Maps OTLP resource attributes to deterministic `EntityID` (SHA-256 of kind + cluster + attributes). Resolution priority: container > pod > node > deployment > statefulset > replicaset > daemonset > namespace > service. Volume metrics with a PVC label resolve to the PVC entity before the generic fallback. |
| `OTLPNormalizer` | `pkg/backend/ingest/normalizer.go` | Performs generic OTLP scalar/histogram traversal, canonical mapping, source-assignment enforcement, conversion application, and sample emission. Histograms are stored as `_sum`/`_count`; delta-valued series are preserved raw with `temporality=delta`. |
| Source dialect registry | `pkg/backend/ingest/source_dialect.go` | Dispatches bounded source-specific behavior to one `source_dialect_*.go` implementation per logical source. Dialects own directional variants, source labels, entity/edge attachment, ResourceQuota units, servicegraph enrichment, and mesh metadata. OTLP temporality handling remains generic and follows the payload contract. |
| `MetricTopologyBuilder` | `pkg/backend/ingest/metric_topology_builder.go` | `runs_on` (pod→node), `contains` (namespace→pod, pod→container) plus topology nodes for those entity kinds. In Helm deployments, `runs_on` additionally requires a live matching Pod UID and node from Kubernetes lifecycle state. |
| `MeshMetricTopologyBuilder` | `pkg/backend/ingest/mesh_metric_topology_builder.go` | `calls` edges with `mesh.istio` provenance from servicegraph-shaped Istio metrics, preserving protocol, mTLS, principal, response-flag, and HTTP/gRPC status labels. Uses datapoint timestamp for `FirstSeen`/`LastSeen`. |
| `TraceTopologyBuilder` | `pkg/backend/ingest/topology_builder.go` | `calls` edges from OTLP spans where `peer.service` is set; deduplicated within a batch by edge ID. |
| `K8sStructuralTopologyBuilder` | `pkg/backend/ingest/structural_topology_builder.go` | `exposes`, `routes_to`, `uses`, `mounts` from `ClusterProfile` (provenance `k8s-api`, confidence 1.0). |
| `CompositeTopologyBuilder` | `pkg/backend/ingest/composite_topology_builder.go` | Chains topology builders so the metric path runs structural and mesh together. |
| `MultiIngestor` | `pkg/backend/ingest/normalizing_ingestor.go` | Fans an envelope to `NormalizingIngestor → MetricStore` and `BufferedTopologyIngestor → GraphStore`. |
| `BufferedTopologyIngestor` | `pkg/backend/ingest/buffered_topology_ingestor.go` | Accumulates edges/nodes from metric and trace builders; flushes one consolidated snapshot to `GraphStore` per flush window (default `OKSG_TOPOLOGY_FLUSH_INTERVAL=60s`). |
| `StructuralTopologyIngestor` | `pkg/backend/ingest/structural_topology_ingestor.go` | Unbuffered ingest path used to seed the graph from a `ClusterProfile` at startup. |

The `--cluster-name` flag (or `OKSG_CLUSTER_NAME`) controls the cluster name
component of every entity ID; default `default`.

## Source Arbitration

When more than one available source declares the same canonical metric (e.g.
`servicegraph.request.count` from both Istio mesh and the OTel `service_graph`
connector), `metrics.Resolve(catalog, registry, profile)` picks one winner
per metric using `PolicyPriority` (lowest `Priority` value wins). Output:

- `Assignment` (canonical metric → winning source ID) — seeds the shared
  source policy used by metric normalization and mesh-edge derivation
- `SelectedSources` — drives the planner's "deploy this receiver/connector?"
  decision

Full pipeline in [metrics-pipeline.md](metrics-pipeline.md).

For the POC, a workload Pod has a supported Istio proxy when `istio-proxy` is
either a classic regular container or a Kubernetes native sidecar init
container with `restartPolicy: Always`. Discovery classifies the mesh as
partial when a non-system controller-owned workload Pod is outside an injected
namespace, explicitly disables injection, or lacks one of those supported proxy
layouts. Kubernetes, Istio control-plane, and OKSG namespaces are excluded.
Ambient mode remains unsupported because it has no per-Pod proxy scrape
contract. A complete mesh owns the service-graph metrics and enables the
node-owned mesh scrape job. A partial mesh disables that scrape globally and
selects trace-derived servicegraph only when traces are available; otherwise it
selects no service-graph producer. This is global source arbitration, not
namespace-scoped coverage or boundary-peer routing.

## Topology Reconstruction

Edge model (Standard tier):

| Source | Relation | Target | Data Source |
|--------|----------|--------|-------------|
| pod | `runs_on` | node | kubeletstats resource attributes |
| namespace | `contains` | pod | kubeletstats resource attributes |
| pod | `contains` | container | kubeletstats resource attributes |
| service | `calls` | service | OTLP trace spans (`peer.service`) or Istio mesh metrics |
| service | `exposes` | workload | K8s API (service selector matching) |
| ingress | `routes_to` | service | K8s API (ingress rules) |
| workload | `uses` | configmap/secret | K8s API (pod spec references) |
| workload | `mounts` | pvc | K8s API (pod spec volumes) |

Workload, ingress, configmap, secret, and PVC nodes are produced **only** by
the structural path, never by OTLP. The backend therefore re-runs the
structural ingester every `OKSG_DISCOVERY_REFRESH_INTERVAL` (default `10m`).
Invariant: `discovery_refresh_interval < node_ttl` (default `OKSG_NODE_TTL=24h`).
Violating it makes structural-only kinds disappear from `/api/v1/graph`
after the TTL window.

`InMemoryGraphStore` accumulates nodes and edges across snapshots and
updates `LastSeen` on re-observed edges. The Postgres `GraphStore` mirrors
this with `ON CONFLICT DO UPDATE SET last_seen = GREATEST(...)`.

## Persistence

Store interfaces (`pkg/backend/persistence/`):

| Interface | Responsibility |
|-----------|----------------|
| `MetricStore` | Write and query canonical metric samples |
| `GraphStore` | Topology snapshots (nodes per snapshot, accumulated edges) |
| `MetadataStore` | Cluster discovery profiles |
| `RawStore` | Raw telemetry envelopes (optional) |
| `EntityRegistry` | Canonical entity records |

`pkg/adapters/storage/factory.go` returns `Stores` based on
`StorageConfig.Backend`:

- `memory` (default) — in-memory implementations, no external deps
- `external` — PostgreSQL (graph, metadata, raw, entities) + VictoriaMetrics (metrics)

PostgreSQL uses `pgx/v5`; schema migrations run on startup via
`CREATE TABLE IF NOT EXISTS`. VictoriaMetrics uses the Prometheus-compatible
HTTP API (`/api/v1/import/prometheus` for write, `/api/v1/query_range` for
read) — no snappy/protobuf dependencies.

Backend storage flags:

| Flag | Env Var | Default |
|------|---------|---------|
| `--storage-backend` | `OKSG_STORAGE_BACKEND` | `memory` |
| `--postgres-dsn` | `OKSG_POSTGRES_DSN` | `""` |
| `--victoriametrics-url` | `OKSG_VICTORIAMETRICS_URL` | `""` |
| `--raw-store-enabled` | `OKSG_RAW_STORE_ENABLED` | `false` |
| `--topology-flush-interval` | `OKSG_TOPOLOGY_FLUSH_INTERVAL` | `60s` |
| `--node-ttl` | `OKSG_NODE_TTL` | `24h` |

## REST API

Versioned under `/api/v1`, served on a dedicated port (default `8080`),
separate from OTLP (`4317`/`4318`) and health (`13133`). The API package
depends only on persistence interfaces — never on concrete storage
implementations.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/status` | System status (nominal/degraded/disconnected), freshness, components, runtime health summary |
| GET | `/api/v1/discovery` | Discovery profile summary |
| GET | `/api/v1/graph` | Full topology snapshot with kind/relation counts and available filters |
| GET | `/api/v1/graph/nodes` | Enriched graph nodes (filter: `kind`, `namespace`) |
| GET | `/api/v1/graph/nodes/{id}` | Node with enriched inbound/outbound edges |
| GET | `/api/v1/graph/edges` | Edges (filter: `source`, `target`, `relation`) |
| GET | `/api/v1/metrics` | Metric samples (filter: `entity_id`, `metric_name`, `edge_*_id`, `from`, `to`) |
| GET | `/api/v1/metrics/series` | Samples grouped by metric name with catalog metadata |
| GET | `/api/v1/metrics/catalog` | Canonical metric definitions (filter: `category`) |
| GET | `/api/v1/metrics/catalog/stats` | Per-source catalog sample counts and last-seen timestamps |
| GET | `/api/v1/entities` | Entity list (filter: `kind`, `namespace`) |
| GET | `/api/v1/entities/{id}` | Single entity |

All list endpoints support cursor-based pagination (`page_size`,
`next_token`). Query-time derivations (rate windows for counter-derived
metrics, sum/count averaging for histograms, the
`servicegraph.request.error.ratio` formula) are documented in
[api-reference.md](api-reference.md) and [runtime-flow.md](runtime-flow.md).

REST API flags:

| Flag | Env Var | Default |
|------|---------|---------|
| `--api-listen-address` | `OKSG_BACKEND_API_LISTEN_ADDRESS` | `0.0.0.0:8080` |
| `--status-stale-seconds` | `OKSG_BACKEND_STATUS_STALE_SECONDS` | `300` |

## Frontend

`web/` is a React + Vite + TypeScript SPA served by nginx with `/api/*`
reverse-proxied to `oksg-backend:8080`. Three views:

- `/` Cluster Overview — KPI row, entity breakdown, relation types, runtime health summary, discovery sources
- `/topology` Topology Graph — React Flow canvas with three projection modes (Ownership, Infrastructure, Traffic) and three granularity levels; per-mode layout strategies under `web/src/views/TopologyGraph/layouts/` are selected via `LAYOUT_REGISTRY` (custom tree for Ownership, `@dagrejs/dagre` for Infrastructure, `elkjs` layered orthogonal for Traffic)
- `/metrics` Metrics Explorer — Entity/Edge/Catalog mode toggle, ⌘K global entity search, breadcrumb/builder hybrid filter (entity), edge selector (edge), selection summary with segmented time range, applicability-gated category tabs, restyled MetricCard charts (gridlines + value/unit + info tooltip), raw samples table, and a catalog table with recent source coverage badges

State: TanStack Query for polling, Zustand for cross-view selection.
Theming: CSS custom properties only — no hardcoded hex in components.

Design system, projection-mode rules, and layout details:
[frontend-design-spec.md](frontend-design-spec.md).

## Deployment Model

The Helm chart in `deploy/helm/oksg-otel/` owns Kubernetes resource shape
(namespace, agent ConfigMap + ServiceAccount + RBAC + DaemonSet, gateway
ConfigMap + ServiceAccount + Service + Deployment, backend Service +
Deployment, frontend Service + Deployment + ConfigMap, optional storage
subcharts). The planner produces collector configuration embedded into
release values; the chart renders final resources. Do not reintroduce ad
hoc manifest rendering in Go.

PostgreSQL (Bitnami) and VictoriaMetrics (single-node) are subchart
dependencies, gated by `storage.postgresql.enabled` /
`storage.victoriametrics.enabled`. When `storage.backend=external` and the
subcharts are enabled, the chart auto-derives DSN and VM URL from the
subchart service names; explicit overrides go in `storage.postgresql.dsn`
and `storage.victoriametrics.url`. After modifying `Chart.yaml`
dependencies, run `helm dependency build deploy/helm/oksg-otel/`.

## Configuration Model

- **Helm values** — operator-facing deployment settings: image, namespace, enabled signal classes, OTLP endpoints, log levels
- **ConfigMaps** — structured collector configuration for agent and gateway
- **Secrets** — credentials and sensitive settings
- **Env vars** — runtime toggles and pod/node metadata only

## Logging Policy

OKSG-owned components log structured JSON to stdout only. Required fields:
`ts`, `level`, `component`, `msg`. Common context: `cluster`, `namespace`,
`mode`, `error`. Debugging path: `kubectl logs`. OTel collector workloads
are also configured for stdout JSON; level is set per component through
Helm values.

## Design Principles

1. Keep the design minimal and easy to change.
2. Preserve explicit contracts between discovery, planning, deployment, and graph layers.
3. Let discovery decide what to collect, not how Kubernetes YAML is shaped.
4. Keep technology-specific adapters isolated from domain packages.
5. Remove stale deployment paths and outdated docs when replacing them.
