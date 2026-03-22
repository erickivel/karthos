# Architecture Overview

OKSG transforms discovered observability signals into a canonical graph-oriented foundation. The currently implemented slice covers discovery-driven OpenTelemetry collection plane assembly, backend OTLP ingest, Layer 3 semantic normalization with in-memory persistence, and initial Layer 4 topology reconstruction.

## Current Implemented Boundary

The implemented control flow is:

```text
Kubernetes API -> ClusterProfile -> SourcePlanner -> Helm Values -> Agent/Gateway/Backend Helm Release
```

What is real today:

- `pkg/adapters/kubernetes` discovers cluster resources, Prometheus annotation-based targets, and OTLP-capable workloads
- `pkg/adapters/otel` plans collector behavior from the discovery profile, including hostmetrics, k8scluster, spanmetrics, and servicegraph
- `pkg/adapters/otlp` exposes OTLP gRPC and HTTP ingest for the backend runtime
- `pkg/collector/configgen` converts the plan into typed Helm values
- `pkg/collector/deployer` runs Helm lint/template/upgrade-install
- `pkg/backend/ingest` converts OTLP telemetry into raw envelopes, routes them by signal type, normalizes them into canonical entities and metrics, and extracts structural topology edges from metric resource attributes
- `pkg/backend/persistence` stores canonical metrics and topology nodes and edges in thread-safe in-memory stores
- `cmd/backend` runs the backend OTLP ingest and normalization service
- `cmd/discovery-job` orchestrates discover, plan, and deploy modes

What is not implemented yet:

- `cmd/controller` is still a placeholder
- query APIs over stored metrics and topology
- durable persistence beyond in-memory stores
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
  Multi-ingestor fan-out for metric signal path
  Graph node accumulation in InMemoryGraphStore

Future Layers
  query and control APIs
  durable persistence
  modeling and prediction
```

## Layer 3: Semantic Normalization

### 3a: Collection Plane Expansion

The source planner now configures expanded receivers beyond the baseline `kubeletstats`:

- `hostmetrics` receiver: always active; collects host-level CPU, memory, disk I/O, network I/O, and load metrics from each node
- `k8scluster` receiver: always active; collects Kubernetes state metrics including replica counts, pod phases, and node conditions
- `kubeletstats` expanded: now includes volume metrics in addition to pod and container stats
- `spanmetrics` connector: enabled when OTLP traces are present; derives request rate and latency distributions from span data
- `servicegraph` connector: enabled when OTLP traces are present; derives service-to-service call edges from spans

The Kubernetes discovery adapter now auto-detects OTLP-capable workloads via:

- annotation `oksg.io/otlp-traces: "true"` on services
- ports 4317 (gRPC) or 4318 (HTTP) present in service specs

### 3b: Semantic Normalization Runtime

The backend normalization pipeline introduced in this layer consists of:

- `OTLPEntityResolver`: maps OTLP semantic convention resource attributes (`k8s.container.name`, `k8s.pod.name`, `k8s.namespace.name`, `k8s.node.name`, `service.name`) to canonical `EntityID` values. Resolution priority is container > pod > node > service. Entity IDs are deterministic, computed via SHA-256 of entity kind, cluster name, and available Kubernetes attributes.
- `OTLPNormalizer`: maps raw OTLP metric payloads to canonical `MetricSample` values using the `OTLPToCanonicalMappings` registry. Applies unit conversion factors and resolves directional variants for disk and network metrics. Metrics with no registered mapping are silently skipped.
- `TraceTopologyBuilder`: extracts service-to-service topology edges from OTLP trace envelopes. Edges are identified by the `peer.service` span attribute. Deduplicates edges within a batch by edge ID; propagates `FirstSeen`/`LastSeen` from span timestamps.
- `NormalizingIngestor`: implements the `Ingestor` interface for metric envelopes; routes them through `OTLPNormalizer` and writes resulting `MetricSample` values to `MetricStore`.
- `TopologyIngestor`: implements the `Ingestor` interface for trace envelopes; routes them through `TraceTopologyBuilder` and writes resulting `topology.Snapshot` values to `GraphStore`.
- `InMemoryMetricStore`: thread-safe in-memory `MetricStore` implementation. Supports write and range queries filtered by entity IDs, metric name, and time window.
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
  ├── NormalizingIngestor  → InMemoryMetricStore
  └── TopologyIngestor(MetricTopologyBuilder) → InMemoryGraphStore
```

### Graph Node Accumulation

`InMemoryGraphStore` now accumulates `topology.Node` values by entity ID alongside edges. Nodes are merged from each `PutSnapshot` call; `GetLatest` returns accumulated nodes and edges together. This allows the graph to reflect the full entity population across multiple telemetry batches.

### Runtime Inspection

The backend exposes a `/debug/graph` endpoint on the health port (13133) that returns the current topology snapshot as JSON, including both `nodes` and `edges`. Use `task test:e2e:debug:graph` to inspect it.

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
