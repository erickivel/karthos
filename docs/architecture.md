# Architecture Overview

OKSG transforms discovered observability signals into a canonical graph-oriented foundation. The currently implemented slice covers discovery-driven OpenTelemetry collection plane assembly, backend OTLP ingest, Layer 3 semantic normalization, Layer 4 topology reconstruction, Layer 5 durable persistence with PostgreSQL and VictoriaMetrics, Layer 6 Phase A REST API for querying graph state, metrics, entities, and system status, and Layer 6 Phase B frontend UI (React dashboard with topology graph, metrics explorer, and cluster overview).

## Current Implemented Boundary

The implemented control flow is:

```text
Kubernetes API -> ClusterProfile -> SourcePlanner -> Helm Values -> Agent/Gateway/Backend Helm Release
```

What is real today:

- `pkg/adapters/kubernetes` discovers cluster resources, Prometheus annotation-based targets, and OTLP-capable workloads
- `pkg/adapters/otel` plans collector behavior from the discovery profile, including hostmetrics and k8scluster on the agent, Istio mesh Prometheus scraping on the agent, plus spanmetrics and servicegraph on the gateway
- `pkg/adapters/otlp` exposes OTLP gRPC and HTTP ingest for the backend runtime
- `pkg/collector/configgen` converts the plan into typed Helm values
- `pkg/collector/deployer` runs Helm lint/template/upgrade-install
- `pkg/backend/ingest` converts OTLP telemetry into raw envelopes, routes them by signal type, normalizes them into canonical entities and metrics, and extracts structural topology edges from metric resource attributes
- `pkg/backend/persistence` defines store interfaces (`MetricStore`, `GraphStore`, `MetadataStore`, `RawStore`, `EntityRegistry`) with in-memory implementations for tests
- `pkg/adapters/storage` provides a factory that wires either in-memory or external (PostgreSQL + VictoriaMetrics) backends based on config
- `pkg/adapters/storage/postgresql` implements `GraphStore`, `MetadataStore`, `RawStore`, and `EntityRegistry` using `pgx/v5`
- `pkg/adapters/storage/victoriametrics` implements `MetricStore` using VictoriaMetrics' Prometheus-compatible HTTP API
- `pkg/adapters/api` provides the Layer 6 Phase A REST API server for querying graph state, metrics, entities, and system status
- `cmd/backend` runs the backend OTLP ingest, normalization, and REST API services with configurable storage backend
- `cmd/discovery-job` orchestrates discover, plan, and deploy modes

What is not implemented yet:

- `cmd/controller` is still a placeholder
- modeling and prediction

## Layer View

```text
Cluster Integration
  Kubernetes discovery adapter

Collection Planning
  Source planner
  Collector config assembly

Collection Runtime
  Agent collector (Layer 3a expanded receivers)
  Gateway collector
  Backend OTLP ingest service

Deployment Packaging
  Helm values builder
  Helm chart
  Helm release installer

Semantic Normalization (Layer 3)
  Entity resolver
  OTLP metric normalizer
  Trace topology builder
  In-memory metric and graph stores

Topology Reconstruction (Layer 4)
  Metric topology builder (structural edges from resource attributes)
  Trace topology builder (service-to-service call edges from spans)
  K8s structural topology builder (exposes, routes_to, uses, mounts from discovery profile)
  Multi-ingestor fan-out for metric signal path
  Graph node accumulation in GraphStore

Persistence (Layer 5)
  Generic store interfaces (MetricStore, GraphStore, MetadataStore, RawStore, EntityRegistry)
  Storage factory with memory and external backends
  PostgreSQL adapter (graph, entities, metadata, raw envelopes) via pgx/v5
  VictoriaMetrics adapter (time-series metrics) via Prometheus HTTP API
  Helm chart storage configuration (backend, DSN secret, VM URL)

Query and Control (Layer 6 Phase A)
  REST API server on dedicated port (8080)
  Graph query endpoints (full graph, nodes, edges, node detail)
  Metrics query and catalog endpoints
  Entity query endpoints
  System status and discovery profile endpoints

Visualization (Layer 6 Phase B)
  React frontend in web/ (Vite + TypeScript)
  Three views: Cluster Overview (/), Topology Graph (/topology), Metrics Explorer (/metrics)
  React Flow graph with dagre layout, custom EntityNode components, and cable-bundle RelationEdge rendering
  TanStack Query polling (30s graph/status, 120s discovery)
  Zustand for cross-view state (selected entity, connection status)
  CSS custom properties theme system (dark theme, no hardcoded colors)
  Docker-only development (Dockerfile.dev + Vite HMR)
  Production: multi-stage build → nginx with /api/* proxy to backend
  Helm templates: frontend-deployment, frontend-service, frontend-configmap

Future Layers
  modeling and prediction
```

## Layer 3: Semantic Normalization

### 3a: Collection Plane Expansion

The source planner now configures expanded receivers beyond the baseline `kubeletstats`:

- `hostmetrics` receiver: always active; collects host-level CPU, memory, disk I/O, network I/O, and load metrics from each node
- `k8scluster` receiver: always active; collects Kubernetes state metrics including replica counts, pod phases, and node conditions
- `kubeletstats` expanded: now includes volume metrics in addition to pod and container stats
- `spanmetrics` connector: enabled on the gateway when OTLP traces are present; derives request rate and latency distributions from span data
- `servicegraph` connector: enabled on the gateway when OTLP traces are present; derives service-to-service call edges from spans
- `mesh-istio` Prometheus scrape job: enabled on the agent when Istio sidecar mode is detected; scrapes `istio-proxy` metrics at `/stats/prometheus`, rewrites Istio labels into `client`/`server` edge attributes, and stamps Prometheus-safe `oksg_source=mesh.istio` (normalized to `source=mesh.istio` in backend samples)

Placing `spanmetrics` and `servicegraph` on the gateway keeps a single shared connector instance for cluster traffic aggregation. That avoids per-agent duplication and double-counting while still letting the agents forward raw OTLP data upstream.

The Kubernetes discovery adapter now auto-detects OTLP-capable workloads via:

- annotation `oksg.io/otlp-traces: "true"` on services
- ports 4317 (gRPC) or 4318 (HTTP) present in service specs

### 3b: Semantic Normalization Runtime

The backend normalization pipeline introduced in this layer consists of:

- `OTLPEntityResolver`: maps OTLP semantic convention resource attributes to canonical `EntityID` values. Resolution priority is container > pod > node > deployment > statefulset > replicaset > daemonset > namespace > service. Workload entities (deployment, statefulset, replicaset, daemonset) are resolved from `k8s_cluster` receiver attributes (`k8s.deployment.name`, `k8s.statefulset.name`, `k8s.replicaset.name`, `k8s.daemonset.name`). Namespace-scoped metrics with no more specific entity resolve to namespace. Entity IDs are deterministic, computed via SHA-256 of entity kind, cluster name, and available Kubernetes attributes.
- `OTLPNormalizer`: translates source-specific metric dialects into semconv-style names first, then emits semconv-native `MetricSample` values using the `OTLPSemconvMetrics` registry. Applies unit conversion factors and resolves directional variants for disk, network, and node condition metrics. Servicegraph connector metrics are enriched with `edge_client`/`edge_server` labels plus stable `edge_client_id`/`edge_server_id` labels for edge-aware querying. Metrics with no registered mapping are silently skipped.
- `TraceTopologyBuilder`: extracts service-to-service topology edges from OTLP trace envelopes. Edges are identified by the `peer.service` span attribute. Deduplicates edges within a batch by edge ID; propagates `FirstSeen`/`LastSeen` from span timestamps.
- `NormalizingIngestor`: implements the `Ingestor` interface for metric envelopes; routes them through `OTLPNormalizer` and writes resulting `MetricSample` values to `MetricStore`.
- `BufferedTopologyIngestor`: implements the `Ingestor` interface for both metric and trace topology paths. Accumulates edges and nodes in memory and flushes one consolidated `topology.Snapshot` to `GraphStore` on a configurable interval (default 60s), reducing per-envelope snapshot writes to one per flush window.
- `TopologyIngestor`: unbuffered topology ingestor retained for use cases that require immediate snapshot writes (e.g. structural topology at startup).
- `InMemoryMetricStore`: thread-safe in-memory `MetricStore` implementation. Supports write and range queries filtered by entity IDs, edge source/target IDs, metric name, and time window.
- `InMemoryGraphStore`: thread-safe in-memory `GraphStore` implementation. Accumulates edges across snapshots; updates `LastSeen` on re-observed edges. Supports edge queries filtered by source or target entity ID.

The `--cluster-name` flag on `cmd/backend` controls the cluster name used for entity ID computation and defaults to `default` (overridable via `OKSG_CLUSTER_NAME`).

## Layer 4: Topology Reconstruction

Layer 4 merges discovered structure with runtime interaction evidence to build temporal graph nodes and edges.

### Structural Edges from Metric Resource Attributes

`MetricTopologyBuilder` extracts structural edges from OTLP metric envelope resource attributes without parsing the metric payload itself. It derives:

- `runs_on`: pod → node (from `k8s.pod.name` + `k8s.node.name` + `k8s.namespace.name`)
- `contains` (namespace → pod): from `k8s.namespace.name` + `k8s.pod.name`
- `contains` (pod → container): from `k8s.pod.name` + `k8s.container.name`

It also implements the optional `NodeBuilder` interface, returning `topology.Node` values for each entity kind it resolves (namespace, node, pod, container).

### MultiIngestor Fan-out

`MultiIngestor` implements the `Ingestor` interface and fans metric envelopes out to multiple downstream ingestors in order. All ingestors receive every envelope; the first error is returned. In `cmd/backend`, the metric signal path uses:

```
MultiIngestor
  ├── NormalizingIngestor  → MetricStore
  └── BufferedTopologyIngestor → GraphStore (flushed periodically)
```

### Graph Node Accumulation

`InMemoryGraphStore` now accumulates `topology.Node` values by entity ID alongside edges. Nodes are merged from each `PutSnapshot` call; `GetLatest` returns accumulated nodes and edges together. This allows the graph to reflect the full entity population across multiple telemetry batches.

### K8s Structural Topology from Discovery Profile

`K8sStructuralTopologyBuilder` derives topology edges from Kubernetes structural data in a `ClusterProfile`, complementing the runtime edges from metrics and traces. It produces:

- `exposes`: service → workload (matched via service selector against workload pod template labels)
- `routes_to`: ingress → service (from ingress rule backend references)
- `uses`: workload → configmap/secret (from pod spec volumes, envFrom, and env valueFrom)
- `mounts`: workload → PVC (from pod spec PVC volume claims)

All edges carry provenance `"k8s-api"` and confidence 1.0. The builder is invoked at backend startup via the `--discovery-profile` flag, which loads a ClusterProfile JSON file and seeds the graph store with structural edges.

The discovery adapter has been extended to capture:

- Ingress resources with backend service references
- Deployment pod template labels (for accurate service selector matching)
- ConfigMap, Secret, and PVC references from deployment pod specs

New entity kinds: `configmap`, `secret`, `pvc`. New relation types: `exposes`, `uses`, `mounts` (plus the previously reserved `routes_to`).

### Standard Tier Graph Model

The graph model follows the "Standard" tier from the architecture spec:

| Source | Relation | Target | Data Source |
|--------|----------|--------|-------------|
| pod | runs_on | node | kubeletstats resource attributes |
| namespace | contains | pod | kubeletstats resource attributes |
| pod | contains | container | kubeletstats resource attributes |
| service | calls | service | OTLP trace spans (peer.service) |
| service | exposes | workload | K8s API (service selector matching) |
| ingress | routes_to | service | K8s API (ingress rules) |
| workload | uses | configmap/secret | K8s API (pod spec references) |
| workload | mounts | pvc | K8s API (pod spec volumes) |

### Runtime Inspection

The backend exposes a `/debug/graph` endpoint on the health port (13133) that returns the current topology snapshot as JSON, including both `nodes` and `edges`. Use `task test:e2e:debug:graph` to inspect it.

## Layer 5: Persistence

Layer 5 makes telemetry and topology data durable by replacing in-memory stores with external backends while keeping technology-specific code behind generic interfaces.

### Interface-First Design

All persistence logic flows through technology-agnostic interfaces in `pkg/backend/persistence/`:

| Interface | Responsibility |
|-----------|---------------|
| `MetricStore` | Write and query canonical time-series metric samples |
| `GraphStore` | Store topology snapshots (nodes per snapshot, accumulated edges) |
| `MetadataStore` | Store and retrieve cluster discovery profiles |
| `RawStore` | Write and query raw telemetry envelopes (optional) |
| `EntityRegistry` | Upsert and query canonical entity records |

Each interface has an in-memory implementation for unit tests and an external implementation for production.

### Storage Factory

`pkg/adapters/storage/factory.go` provides a `NewStores()` factory that creates the appropriate implementations based on `StorageConfig.Backend`:

- `memory` (default): all in-memory implementations, no external dependencies
- `external`: PostgreSQL for graph/entities/metadata/raw, VictoriaMetrics for metrics

The factory validates required config (DSN, VM URL), creates connection pools, runs schema migrations, and registers closers for cleanup.

### PostgreSQL Adapter

`pkg/adapters/storage/postgresql/` implements four store interfaces using `pgx/v5`:

- **GraphStore**: topology snapshots in `topology_snapshots` + `topology_nodes` tables; accumulated edges in `topology_edges` with `ON CONFLICT DO UPDATE SET last_seen = GREATEST(...)` to mirror in-memory merge semantics
- **MetadataStore**: cluster profiles stored as JSONB in `cluster_profiles`, latest retrieved by `created_at DESC`
- **RawStore**: raw envelopes batch-inserted with `ON CONFLICT DO NOTHING`
- **EntityRegistry**: entities upserted with label and last_seen updates on conflict

Schema migrations run on startup via `CREATE TABLE IF NOT EXISTS` / `CREATE INDEX IF NOT EXISTS`.

### VictoriaMetrics Adapter

`pkg/adapters/storage/victoriametrics/` implements `MetricStore` using VictoriaMetrics' Prometheus-compatible HTTP API:

- **Write**: converts `[]MetricSample` to Prometheus text format, POSTs to `/api/v1/import/prometheus`
- **Query**: builds PromQL from `MetricQuery`, GETs `/api/v1/query_range`, parses JSON response

Uses only `net/http` — no snappy/protobuf dependencies.

### Backend Configuration

The backend (`cmd/backend`) supports storage config via CLI flags:

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--storage-backend` | `OKSG_STORAGE_BACKEND` | `memory` | Storage backend to use |
| `--postgres-dsn` | `OKSG_POSTGRES_DSN` | `""` | PostgreSQL connection string |
| `--victoriametrics-url` | `OKSG_VICTORIAMETRICS_URL` | `""` | VictoriaMetrics base URL |
| `--raw-store-enabled` | `OKSG_RAW_STORE_ENABLED` | `false` | Enable raw envelope storage |
| `--topology-flush-interval` | `OKSG_TOPOLOGY_FLUSH_INTERVAL` | `60s` | Topology snapshot flush interval |

### Helm Storage Configuration

The Helm chart includes PostgreSQL (Bitnami) and VictoriaMetrics (single-node) as optional subchart dependencies, gated by condition flags. When enabled, the subcharts deploy and manage the databases; the DSN and VM URL are auto-derived from the subchart service names.

```yaml
storage:
  backend: memory           # "memory" or "external"
  rawStoreEnabled: false
  postgresql:
    enabled: false           # deploy Bitnami PostgreSQL subchart
    dsn: ""                  # override: use explicit DSN instead of subchart
  victoriametrics:
    enabled: false           # deploy VictoriaMetrics subchart
    url: ""                  # override: use explicit URL instead of subchart

# Subchart value overrides
postgresql:
  auth:
    database: oksg
    username: oksg
    password: oksg

victoria-metrics-single:
  server:
    persistentVolume:
      size: 5Gi
    retentionPeriod: 30d
```

When `storage.backend=external`, the chart creates a `storage-secret` with the PostgreSQL DSN and passes all storage env vars to the backend deployment. The DSN and VM URL are auto-derived from subchart service names unless explicitly overridden via `storage.postgresql.dsn` or `storage.victoriametrics.url`.

After modifying `Chart.yaml` dependencies, run `helm dependency build deploy/helm/oksg-otel/` to download the subchart tarballs.

## Layer 6: REST API

The REST API server runs on a dedicated port (default 8080) within the backend binary, separate from OTLP ingest (4317/4318) and health check (13133) ports.

The API package (`pkg/adapters/api/`) depends only on persistence interfaces -- it never imports concrete storage implementations. The storage backend can be swapped without touching API code. Graph handlers cross-reference the EntityRegistry to enrich node responses with display-friendly metadata (name, namespace, cluster, status).

### Endpoints

All endpoints are versioned under `/api/v1/` and return JSON with a consistent envelope. List endpoints support cursor-based pagination (`page_size`, `next_token`).

| Method | Path | Description |
|--------|------|-------------|
| GET | `/api/v1/status` | System status (nominal/degraded/disconnected), freshness, components |
| GET | `/api/v1/discovery` | Discovery profile summary with entity counts |
| GET | `/api/v1/graph` | Full topology snapshot with kind/relation counts and available filters |
| GET | `/api/v1/graph/nodes` | Enriched graph nodes (filter: `kind`, `namespace`) with pagination |
| GET | `/api/v1/graph/nodes/{id}` | Single node with enriched edges including related entity summaries |
| GET | `/api/v1/graph/edges` | Edges (filter: `source`, `target`, `relation`) with pagination |
| GET | `/api/v1/metrics` | Metric samples with unit (filter: `entity_id`, `metric_name`, `from`, `to`) |
| GET | `/api/v1/metrics/series` | Samples grouped by metric name with catalog metadata |
| GET | `/api/v1/metrics/catalog` | Canonical metric definitions (filter: `category`) |
| GET | `/api/v1/entities` | Entity list (filter: `kind`, `namespace`) with pagination |
| GET | `/api/v1/entities/{id}` | Get single entity by ID |

### Configuration

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--api-listen-address` | `OKSG_BACKEND_API_LISTEN_ADDRESS` | `0.0.0.0:8080` | REST API listen address |
| `--status-stale-seconds` | `OKSG_BACKEND_STATUS_STALE_SECONDS` | `300` | Snapshot staleness threshold |

## Deployment Model

The Helm chart in `deploy/helm/oksg-otel` owns the Kubernetes resource shape:

- namespace creation
- agent ConfigMap
- gateway ConfigMap
- agent ServiceAccount and RBAC
- agent DaemonSet
- gateway ServiceAccount
- gateway Service
- gateway Deployment
- backend Service
- backend Deployment

The planner no longer renders Kubernetes objects directly. It produces collector configuration that is embedded into release values, and Helm renders the final resources.

## Configuration Model

Configuration is split intentionally:

- Helm values:
  operator-facing deployment settings such as release name, image settings, namespace, enabled signal classes, OTLP endpoints, and log levels
- ConfigMaps:
  structured collector configuration for agent and gateway
- Secrets:
  reserved for credentials and sensitive settings
- Env vars:
  runtime toggles and pod or node metadata only

## Logging Policy

OKSG-owned runtime components use structured JSON logs to stdout only.

- required fields: `ts`, `level`, `component`, `msg`
- common context when available: `cluster`, `namespace`, `mode`, `error`
- debugging path: `kubectl logs`

OTel collector workloads are also configured for stdout-based logging, with per-component log level controlled through Helm values.

The backend OTLP ingest runtime logs structured lines for pipeline startup and per-request events; end-to-end verification uses `kubectl logs`.

## Design Principles Still In Force

1. Keep the design minimal and easy to change.
2. Preserve explicit contracts between discovery, planning, deployment, and future graph layers.
3. Let discovery decide what to collect, not how Kubernetes YAML is shaped.
4. Keep technology-specific adapters isolated from domain packages.
5. Remove stale deployment paths and outdated docs when replacing them.
