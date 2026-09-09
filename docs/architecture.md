# Architecture Overview

Karthos turns Kubernetes discovery and OpenTelemetry-driven runtime telemetry
into a queryable system graph. Control flow:

```text
Kubernetes API -> ClusterProfile -> Source Plan -> Generic Collection Plan -> OTel Adapter -> Generated Helm Values
                                                                                                      |                |
                                                                                                      |                v
                                                                                                      |       Helm source-contract ConfigMap
                                                                                                      v                |
                                                                                                 Helm Release ---------+
                                                                        |
                                                                        v
                              Backend (strict contract validation, OTLP ingest, normalization, topology, REST API)
                                                                        |
                                                                        v
                                                              React frontend (web/)
```

## Components

| Path | Role |
|------|------|
| `pkg/adapters/kubernetes` | Kubernetes API inventory adapter plus live Pod lifecycle validation — lists discovery observations once and maintains UID/node List/Watch state |
| `pkg/adapters/otel` | OpenTelemetry adapter: renders receiver, connector, processor, pipeline, exporter, and source-stamp syntax from the generic collection plan |
| `pkg/adapters/otlp` | OTLP gRPC ingress server |
| `pkg/adapters/api` | Versioned REST API server (handlers, response envelope, pagination) |
| `pkg/adapters/storage` | Storage factory; `postgresql/` and `victoriametrics/` external implementations |
| `pkg/collector/{spec,configgen,deployer}` | Provider-neutral collection requirements and composition → Helm values → `helm lint/template/upgrade-install` |
| `pkg/backend/ingest` | Envelope build, signal routing, normalization, topology builders, ingestors |
| `pkg/backend/persistence` | Store interfaces and in-memory implementations |
| `pkg/core/metrics` | Canonical catalog, provider-neutral instrument semantics, and resolver contracts — see [metrics-pipeline.md](metrics-pipeline.md) |
| `pkg/core/semconv` | Shared OTLP semantic-convention attribute key constants |
| `pkg/core/{entities,topology,schema}` | Canonical entity / topology / envelope types |
| `pkg/discovery/profile` | Capability-only `ClusterProfile` and bounded source evidence |
| `pkg/sources` | Compiled source-module inventory; each module owns detection, semantic collection requirements, and normalization; `sources/contract` owns the mounted source-plan contract; `structure/` owns direct Kubernetes structural refresh |
| `cmd/discovery-job` | `discover` / `plan` / `deploy` CLI |
| `cmd/backend` | Mounted source-contract validation, OTLP ingest, normalization, REST API, structural refresh loop |
| `web/` | React frontend (Vite + TypeScript) — Cluster Overview, Topology Graph, Metrics Explorer |
| `deploy/helm/karthos` | Helm chart packaging agent, gateway, backend, frontend, optional storage subcharts |

## Collection Plane

Static source modules emit semantic collection requirements. The generic
collector composer aggregates them with the resolved source plan, the OTel
adapter renders OTel-specific configuration, and the Helm values compiler
packages that output with the mounted source contract. Receivers and connectors:

- kubeletstats as component ID `kubelet_stats` (always; pod, container, volume metrics)
- hostmetrics as component ID `host_metrics` (always; per-node CPU, memory, disk, filesystem, and network data). Agent pods use the host network so interface counters describe the node.
- `k8s_cluster` (always on the gateway; an explicit Collector v0.156 allowlist retains only container requests/limits/status, pod phase/reason, and selected node conditions). Its pinned receiver still watches `resourcequotas`, so gateway RBAC retains that permission while the ResourceQuota metric family and graph kind remain disabled. See the [v0.156 receiver RBAC contract](https://raw.githubusercontent.com/open-telemetry/opentelemetry-collector-contrib/v0.156.0/receiver/k8sclusterreceiver/README.md).
- `prometheus` on the singleton gateway for application Service-DNS targets (scrape jobs stamp `karthos_source=prometheus.app`)
- `mesh-istio` Prometheus job (only when discovery finds a complete sidecar mesh; each DaemonSet agent scrapes only `istio-proxy:15020/stats/prometheus` Pods on its own node, rewrites Istio labels into servicegraph `client`/`server` edge attributes, and stamps `karthos_source=mesh.istio`)
- `otlp` receivers (when discovery finds workloads with `karthos.io/otlp-traces: "true"` or ports `4317`/`4318`)
- servicegraph as component ID `service_graph` (gateway, when traces are present and the mesh does not own the POC service-graph coverage; it is the sole selected source for a partial mesh)

Collector-owned agent and gateway pipelines run a `transform/karthos-source`
processor that stamps the Prometheus-safe `karthos_source` datapoint attribute from
the receiver-determined metric-name namespace. The backend accepts either
`karthos_source` or dotted `karthos.source` from producers. External OTLP metrics use isolated `metrics/otlp` pipelines without
inference and must arrive with a registered source. Together with scrape-time relabel stamps
(`karthos_source=prometheus.app` / `mesh.istio`) this gives the backend a
reliable signal for source arbitration.

## Semantic Normalization

The backend ingest pipeline:

| Component | File | Role |
|-----------|------|------|
| `OTLPEntityResolver` | `pkg/backend/ingest/entity_resolver.go` | Maps OTLP resource attributes to deterministic `EntityID` (SHA-256 of kind + cluster + attributes) and persists canonical entity metadata. Resolution priority: container > pod > node > deployment > statefulset > replicaset > daemonset > job > cronjob > replicationcontroller > HPA > namespace > service. Volume metrics with a PVC label resolve to the PVC entity before the generic fallback. |
| `OTLPNormalizer` | `pkg/backend/ingest/normalizer.go` | Performs generic OTLP scalar/histogram traversal after a stamped source parser returns a canonical candidate; validates catalog membership and assignment, applies conversion, and emits samples. Datapoints without a producer timestamp are dropped; histograms are stored as `_sum`/`_count`; delta-valued series are preserved raw with `temporality=delta`. |
| Source modules | `pkg/sources/{kubelet,hostos,kubecluster,prometheusapp,servicegraph,istio}` | Each module owns stamped raw-name parsing, bounded source-specific labels, calls-edge classification/metadata, entity/edge attachment, conversions, and derived metrics through the shared `pkg/sources/contract` boundary. OTLP temporality handling remains generic. |
| `MetricTopologyBuilder` | `pkg/backend/ingest/metric_topology_builder.go` | Observed `runs_on` (pod→node) only. In Helm deployments, it additionally requires a live matching Pod UID and node from Kubernetes lifecycle state. Direct Kubernetes refresh owns structural nodes and membership relations. |
| `MetricCallsTopologyBuilder` | `pkg/backend/ingest/metric_calls_topology_builder.go` | Generic `calls` edge assembly from source-classified metrics. The source module owns recognition and bounded metadata; the builder validates provenance, uses datapoint timestamps for `FirstSeen`/`LastSeen`, and deduplicates edges. |
| `TraceTopologyBuilder` | `pkg/backend/ingest/topology_builder.go` | `calls` edges from OTLP spans where `peer.service` is set; deduplicated within a batch by edge ID. |
| `kubernetes.structure` | `pkg/sources/structure` | Direct Kubernetes API refresh of `exposes`, `routes_to`, `uses`, `mounts`, and containment with `k8s-api` provenance. |
| `CompositeTopologyBuilder` | `pkg/backend/ingest/composite_topology_builder.go` | Chains metric-derived observed `runs_on` and source-classified `calls` builders. Direct Kubernetes structure refresh remains separate. |
| `MultiIngestor` | `pkg/backend/ingest/normalizing_ingestor.go` | Fans an envelope to `NormalizingIngestor → MetricStore` and `BufferedTopologyIngestor → GraphStore`. |
| `BufferedTopologyIngestor` | `pkg/backend/ingest/buffered_topology_ingestor.go` | Accumulates only observed `runs_on` and `calls` edges from metric and trace builders; flushes one consolidated edge batch per window (default `KARTHOS_TOPOLOGY_FLUSH_INTERVAL=60s`) and requeues a failed in-process write for the next flush. Direct Kubernetes refresh owns all structural nodes and relations. |

The `--cluster-name` flag (or `KARTHOS_CLUSTER_NAME`) controls the cluster name
component of every entity ID; default `default`.

## Source Arbitration

When more than one available source declares the same canonical metric (e.g.
`servicegraph.request.count` from both Istio mesh and the OTel `service_graph`
connector), `metrics.Resolve(catalog, registry, profile)` picks the available
source with the lowest `Priority` value for each metric. Output:

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
layouts. Kubernetes, Istio control-plane, and Karthos namespaces are excluded.
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
| namespace | `contains` | pod | Direct Kubernetes structure refresh |
| pod | `contains` | container | Direct Kubernetes structure refresh |
| service | `calls` | service | OTLP trace spans (`peer.service`) or source-classified metric evidence |
| service | `exposes` | workload | K8s API (service selector matching) |
| ingress | `routes_to` | service | K8s API (ingress rules) |
| workload | `uses` | configmap/secret | K8s API (pod spec references) |
| workload | `mounts` | pvc | K8s API (pod spec volumes) |

Workload, ingress, configmap, secret, and PVC nodes are produced **only** by
the direct Kubernetes structure module, never by OTLP. The backend therefore
refreshes that module every `KARTHOS_STRUCTURE_REFRESH_INTERVAL` (default `5m`).
Invariant: `structure_refresh_interval < node_ttl` (default `KARTHOS_NODE_TTL=24h`).
Violating it makes structural-only kinds disappear from `/api/v1/graph`
after the TTL window.

`GraphStore` separates two write paths. The direct Kubernetes structure module
appends complete structural snapshots. Telemetry and Pod lifecycle ingestion
maintain only current observed edges. After the initial refresh and every
`KARTHOS_STRUCTURE_REFRESH_INTERVAL`, the backend materializes one immutable full
graph snapshot: the newest structure provides all nodes and structural edges;
only active observed `runs_on`/`calls` edges seen in the preceding five minutes
survive, and both endpoints must be structural members. The API reads only
these materialized snapshots, which are retained for 24 hours and addressed by
their ID or `?at=<RFC3339>` selection. An object omitted by the next refresh is
absent from the next materialization without rewriting earlier history.

## Persistence

Store interfaces (`pkg/backend/persistence/`):

| Interface | Responsibility |
|-----------|----------------|
| `MetricStore` | Write and query canonical metric samples; provider-neutral output decoration adds public enum labels |
| `GraphStore` | Structural writes, current observed-edge state, and retained materialized graph snapshots |
| `MetadataStore` | Cluster capability profiles |
| `EntityRegistry` | Canonical entity records |

`pkg/adapters/storage/factory.go` returns `Stores` based on
`StorageConfig.Backend`:

- `memory` (default) — in-memory implementations, no external deps
- `external` — PostgreSQL (graph, metadata, entities) + VictoriaMetrics (metrics)

PostgreSQL uses `pgx/v5`; schema migrations run on startup via
`CREATE TABLE IF NOT EXISTS`. The structural-snapshot model requires a fresh
external PostgreSQL schema; it deliberately has no legacy converter or reader.
VictoriaMetrics uses the Prometheus-compatible
HTTP API (`/api/v1/import/prometheus` for write, `/api/v1/query_range` for
read) — no snappy/protobuf dependencies.

Backend storage flags:

| Flag | Env Var | Default |
|------|---------|---------|
| `--storage-backend` | `KARTHOS_STORAGE_BACKEND` | `memory` |
| `--postgres-dsn` | `KARTHOS_POSTGRES_DSN` | `""` |
| `--victoriametrics-url` | `KARTHOS_VICTORIAMETRICS_URL` | `""` |
| `--topology-flush-interval` | `KARTHOS_TOPOLOGY_FLUSH_INTERVAL` | `60s` |
| `--node-ttl` | `KARTHOS_NODE_TTL` | `24h` |

## REST API

Versioned under `/api/v1`, served on a dedicated port (default `8080`),
separate from backend OTLP gRPC (`4317`) and health (`13133`). The API package
depends only on persistence interfaces — never on concrete storage
implementations.

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/status` | System status (nominal/degraded/disconnected), freshness, components, runtime health summary |
| GET | `/api/v1/discovery` | Capability profile summary |
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
histogram means) are documented in
[api-reference.md](api-reference.md) and [runtime-flow.md](runtime-flow.md).

REST API flags:

| Flag | Env Var | Default |
|------|---------|---------|
| `--api-listen-address` | `KARTHOS_BACKEND_API_LISTEN_ADDRESS` | `0.0.0.0:8080` |
| `--status-stale-seconds` | `KARTHOS_BACKEND_STATUS_STALE_SECONDS` | `360` |

## Frontend

`web/` is a React + Vite + TypeScript SPA served by nginx with `/api/*`
reverse-proxied in Helm deployments to the release-scoped backend Service
(`karthos-backend:8080` for the default release). Three views:

- `/` Cluster Overview — KPI row, entity breakdown, relation types, runtime health summary, discovery sources
- `/topology` Topology Graph — React Flow canvas with three projection modes (Ownership, Infrastructure, Traffic) and three granularity levels; per-mode layout strategies under `web/src/views/TopologyGraph/layouts/` are selected via `LAYOUT_REGISTRY` (custom tree for Ownership, `@dagrejs/dagre` for Infrastructure, `elkjs` layered orthogonal for Traffic)
- `/metrics` Metrics Explorer — Entity/Edge/Catalog mode toggle, ⌘K global entity search, breadcrumb/builder hybrid filter (entity), edge selector (edge), selection summary with segmented time range, applicability-gated category tabs, restyled MetricCard charts (gridlines + value/unit + info tooltip), raw samples table, and a catalog table with recent source coverage badges

State: TanStack Query for polling, Zustand for cross-view selection.
Theming: CSS custom properties only — no hardcoded hex in components.

Design system, projection-mode rules, and layout details:
[frontend-design-spec.md](frontend-design-spec.md).

## Deployment Model

The Helm chart in `deploy/helm/karthos/` owns Kubernetes resource shape
(agent ConfigMap + ServiceAccount + RBAC + DaemonSet, gateway
ConfigMap + ServiceAccount + Service + Deployment, one backend source-contract
ConfigMap + Service + Deployment, frontend Service + Deployment + ConfigMap,
optional storage subcharts). The planner produces collector configuration plus
the compiled capability-profile/source-plan contract embedded into a dynamic
values overlay; the chart supplies static policy and renders final resources.
Helm release name and namespace are the only deployment identity. Do not
reintroduce ad hoc manifest rendering in Go.

PostgreSQL (Bitnami) and VictoriaMetrics (single-node) are subchart
dependencies, gated by `storage.postgresql.enabled` /
`storage.victoriametrics.enabled`. When `storage.backend=external` and the
subcharts are enabled, the chart auto-derives DSN and VM URL from the
subchart service names; explicit overrides go in `storage.postgresql.dsn`
and `storage.victoriametrics.url`. After modifying `Chart.yaml`
dependencies, run `helm dependency build deploy/helm/karthos/`.

For development-only telemetry inspection, `discovery-job --collector-debug`
(or `KARTHOS_COLLECTOR_DEBUG=true`) asks the OTel adapter to compile its fixed
`debug/karthos` exporter into every enabled agent and gateway pipeline. It is
disabled by default, writes detailed batches directly to Collector stdout, and
is never a backend persistence path. Both planes intentionally log the same
forwarded telemetry while debugging, so output is temporary and duplicated.
The debug exporter is alpha and its text output is not a stable contract; see
the [Collector v0.156 debug exporter documentation](https://raw.githubusercontent.com/open-telemetry/opentelemetry-collector/v0.156.0/exporter/debugexporter/README.md).
The chart schema requires an explicit `storage.backend` (`memory` or
`external`) and rejects unknown storage settings.

## Configuration Model

- **Helm values** — chart-owned static policy plus a generated dynamic overlay
  for collector configuration, cluster/signal data, source contract, and
  explicit image/storage overrides
- **ConfigMaps** — structured collector configuration for agent and gateway plus the chart-owned backend source contract
- **Secrets** — credentials and sensitive settings
- **Env vars** — pod/node metadata and chart-propagated Collector endpoint
  values; `KARTHOS_BACKEND_GRPC_ENDPOINT` and `KARTHOS_GATEWAY_GRPC_ENDPOINT` are
  generated from the chart's release-scoped Services and reserved from user
  overrides

## Logging Policy

The backend logs structured JSON to stdout only. Required fields: `ts`,
`level`, `component`, `msg`. Common context: `cluster`, `namespace`, `mode`,
`error`. Its Helm-owned `backend.logLevel` accepts `info`, `warn`, or `error`.

The OTel adapter fixes Collector self-telemetry to JSON at `info` on stdout in
both the agent and gateway configuration. It is not a Helm setting. When
enabled for development, the adapter-owned debug exporter emits transient
diagnostic batches to Collector stdout; that output is distinct from the
structured self-telemetry policy. Use `kubectl logs` for both paths.

## Design Principles

1. Keep the design minimal and easy to change.
2. Preserve explicit contracts between discovery, planning, deployment, and graph layers.
3. Let discovery decide what to collect, not how Kubernetes YAML is shaped.
4. Keep technology-specific adapters isolated from domain packages.
5. Remove stale deployment paths and outdated docs when replacing them.
