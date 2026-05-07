# Metrics Pipeline

This document explains how metric signals travel from heterogeneous producers
to a single canonical, semconv-named series. Four pieces work together:

1. **Catalog** — every canonical metric the system knows how to render
2. **Source registry** — the logical producers that can emit a canonical metric
3. **Dialect registry** — name translation from raw producer wire format to canonical
4. **Resolver** — picks one winning source per metric for a given cluster

All four live in `pkg/core/metrics/` and are pure data plus pure functions, so
they are exercised end-to-end in unit tests without a running cluster.

## 1. Canonical Catalog

`pkg/core/metrics/catalog.go` declares every metric the platform exposes
through the API and the UI. `DefaultCatalog()` returns a copy of the in-process
registry; the running list lives in `defaultCatalog`.

Each entry is a `CanonicalMetric` (`pkg/core/metrics/canonical.go`):

```go
type CanonicalMetric struct {
    Name            string          // semconv-style name, e.g. "k8s.pod.cpu.usage"
    DisplayName     string          // operator-facing label
    Unit            Unit            // canonical unit (millicores, bytes, ns, ratio, ...)
    Category        MetricCategory  // resource | traffic | latency | error | capacity | health
    Scopes          []MetricScope   // entity, edge, or both
    Description     string
    Sources         []SourceRef     // {ID, Priority} — which logical sources can produce it
    SelectionPolicy SelectionPolicy // priority (default), disjoint, all
}
```

Important properties:

- **Names are semconv-first.** Producer-specific names (`istio_requests_total`,
  `traces.span.metrics.calls`) never reach storage or the API; they are
  translated by a dialect before normalization.
- **Scopes are authoritative for the API and UI.** A metric in `scopes:["edge"]`
  must only be requested with `edge_source_id` / `edge_target_id` filters; the
  Metrics Explorer enforces this at query time.
- **Sources drive arbitration**, not detection. A metric with several entries
  in `Sources` declares more than one possible producer; the resolver picks
  exactly one per cluster.
- **`servicegraph.request.error.ratio` has no Sources.** It is query-derived
  at the API layer (see `docs/api-reference.md` and `docs/runtime-flow.md`).
- **Catalog ↔ ingest invariant.** Every cataloged metric (other than the
  query-derived exceptions tracked in `catalogSourcelessExceptions`) must
  appear in `OTLPSemconvMetrics` (`pkg/core/metrics/otel_mappings.go`); the
  test `TestDefaultCatalogMetricsHaveIngestOrQuerySupport` enforces this.
- **Unsupported metrics stay out of the catalog.** Names that have no real
  ingest path (e.g. `k8s.pod.phase`, `k8s.pod.network.io.*`,
  `k8s.node.*` capacity gauges) are explicitly excluded by
  `TestDefaultCatalogExcludesUnsupportedMetrics` so the UI never advertises a
  metric the backend cannot produce.

The current categories are `resource`, `traffic`, `latency`, `error`,
`capacity`, and `health`. Categories drive the Metrics Explorer tab layout.

## 2. Logical Source Registry

`pkg/core/metrics/sources.go` declares the producers OKSG knows how to talk
to. `DefaultLogicalSources()` returns the built-in registry:

| ID | DisplayName | Detect | Dialect |
|----|-------------|--------|---------|
| `kubelet` | Kubelet stats | always | `KubeletDialect` |
| `host-os` | Host OS metrics | always | none |
| `kube-cluster` | Kubernetes API | always | none |
| `prometheus.app` | Application Prometheus endpoints | profile has prometheus targets | `PrometheusDialect` |
| `mesh.istio` | Istio service mesh | profile contains an `istio` source with `Mesh != nil` | `IstioDialect` |
| `traces.spanmetrics` | Trace-derived span metrics | profile advertises OTLP traces | `SpanmetricsDialect` |
| `traces.servicegraph` | Trace-derived service graph | profile advertises OTLP traces | `ServicegraphDialect` |

Each entry is a `LogicalSource`:

```go
type LogicalSource struct {
    ID          string
    DisplayName string
    Description string
    Detect      func(*profile.ClusterProfile) bool
    Dialect     Dialect
}
```

`Detect` reads the discovery profile and answers "could this cluster produce
metrics from this source?". `Dialect` describes how to translate raw names.
A `nil` dialect means the source already emits canonical-shaped names. The
kubeletstats receiver emits a mix of `k8s.*` and upstream `container.*` names,
so its source dialect maps container usage metrics onto the public
`k8s.container.*` catalog names.

Source IDs are exported as constants so the catalog, dialect, and planner
share a single contract:

```go
SourceKubelet, SourceHostOS, SourceKubeCluster,
SourcePrometheusApp, SourceMeshIstio,
SourceTracesSpanmetrics, SourceTracesServicegraph
```

## 3. Dialect Translation

`pkg/core/metrics/dialect.go` defines:

```go
type Dialect interface {
    Translate(metricName string) (string, bool)
}
```

A dialect is a `map[rawName]canonicalName`. The built-in dialects:

| Dialect | Example translation |
|---------|---------------------|
| `PrometheusDialect` | `http_requests_total` → `http.server.request.count`, `http_request_duration_seconds` → `http.server.request.duration` |
| `KubeletDialect` | `container.cpu.usage` → `k8s.container.cpu.usage`, `container.memory.usage` → `k8s.container.memory.usage` |
| `SpanmetricsDialect` | `traces.span.metrics.calls` → `rpc.server.request.count`, `traces.span.metrics.duration` → `rpc.server.duration` |
| `ServicegraphDialect` | `traces.service.graph.request.total` → `servicegraph.request.count`, `traces.service.graph.request.server` → `servicegraph.request.duration`, `traces.service.graph.request_failed_total` → `servicegraph.request.error.count` |
| `IstioDialect` | `istio_requests_total` → `servicegraph.request.count`, `istio_request_duration_milliseconds` → `servicegraph.request.duration`, plus body size, gRPC message, and TCP connection counters |

`DialectRegistry` chains dialects in the order their owning sources were
registered; first match wins. Unknown names pass through unchanged so
canonical-shaped metrics are not corrupted.

`NewDialectRegistryFromSources(sources)` is the preferred constructor at
startup: it walks the source registry and collects the non-nil dialects in
order, so any change to the source list updates the dialect chain
automatically.

A dialect only renames the metric — it does **not** touch units, datapoint
attributes, or histogram shape. Unit conversion and label rewriting happen
in `OTLPNormalizer` (`pkg/backend/ingest/normalizer.go`) after dialect
translation.

## 4. Resolver and Arbitration

When more than one available source declares the same canonical metric (for
example Istio mesh + servicegraph connector both produce
`servicegraph.request.count`), exactly one source must win or the metric
will double-count.

`pkg/core/metrics/resolver.go`:

```go
func Resolve(catalog []CanonicalMetric, registry []LogicalSource,
             prof *profile.ClusterProfile) (Assignment, SelectedSources, error)
```

- `Assignment` is `map[canonicalName]sourceID` — one winner per metric.
- `SelectedSources` is the set of source IDs that won at least one metric.
- Default `SelectionPolicy` is `PolicyPriority`: lowest `Priority` value wins
  among `Detect`-eligible sources. `PolicyDisjoint` and `PolicyAll` are
  reserved for future per-scope or aggregate routing and currently return an
  error so unimplemented behavior cannot ship silently.
- A metric with no eligible source is dropped from the assignment (best-effort
  missing data, not an error).

The output drives two consumers:

### 4a. Planner (`pkg/adapters/otel/source_planner.go`)

The planner deploys only receivers/connectors whose source ID appears in
`SelectedSources`. For example, when Istio is present and wins all
`servicegraph.*` metrics, the gateway omits the `servicegraph` connector
entirely. The planner also installs a `transform/oksg-source` processor on
agent and gateway pipelines that stamps `oksg_source` on every datapoint
based on its receiver-determined metric-name namespace. Application
Prometheus and Istio scrape jobs also stamp `oksg_source` at scrape time via
`relabel_configs`, using the Prometheus-safe `oksg_source` label that the
backend later normalizes to the `source` attribute.

### 4b. Ingester (`pkg/backend/ingest/normalizer.go`)

`OTLPNormalizer.WithAssignment(assignment)` activates source-mismatch
filtering. After dialect translation and directional name resolution, the
normalizer compares each datapoint's `oksg_source` attribute against
`Assignment[canonical]`. Mismatches are dropped and counted via
`OTLPNormalizer.DroppedSampleCount()` (an `atomic.Uint64` for safety under
the OTLP server's concurrent request handling). Untagged samples follow a
legacy accept path so envelopes from external producers are not silently
dropped.

This is the mechanism that prevents double-counting when the same canonical
metric has two installed producers.

## 5. Ingest-Side Mappings

`pkg/core/metrics/otel_mappings.go` is the OTLP-side companion to the
catalog. `OTLPSemconvMetrics` maps each canonical metric name to:

- `ConversionFactor` — from the producer's unit to the canonical unit
- `Unit` — canonical unit code, used by the API response
- `Kind` — gauge, sum, or histogram

The normalizer reads this map after dialect translation to apply unit
conversion and to choose the right datapoint extraction path (scalar values
for gauges/sums, `_sum`/`_count` decomposition for histograms, average
extraction for non-quantile latency reads, etc.).

`pkg/core/semconv/keys.go` centralizes the OTLP attribute keys used across
ingest and topology builders (`k8s.pod.name`, `peer.service`,
`k8s.persistentvolumeclaim.name`, mesh-specific keys, the `oksg.source` and
`oksg_source` attribute names, etc.). Updating an attribute key happens in
one place and any code that imports `pkg/core/semconv` benefits.

## 6. End-to-End Example: `servicegraph.request.count`

This metric is the canonical "request rate between two services or workloads"
counter. It can be produced by two sources:

- `mesh.istio` (priority 1) — sidecar `istio_requests_total`
- `traces.servicegraph` (priority 2) — OTel `traces.service.graph.request.total`

Pipeline for an Istio cluster:

1. **Discovery** detects `istio-system`, `istio-sidecar-injector`, and
   produces an `ObservabilitySource{Type:"istio", Mesh:{...}}`.
2. **Resolver** sees both `mesh.istio` (`hasIstioMesh` is true) and
   `traces.servicegraph` (`hasTraces` is true). PolicyPriority picks
   `mesh.istio`. `SelectedSources` = `{kubelet, host-os, kube-cluster,
   mesh.istio, traces.spanmetrics, traces.servicegraph}` if traces are also
   on, but `Assignment["servicegraph.request.count"] = "mesh.istio"`.
3. **Planner** does **not** wire the `servicegraph` connector (because
   nothing in the assignment routes through it, and the planner consults the
   full assignment, not just `SelectedSources`, for connector-vs-receiver
   decisions). It does configure the `mesh-istio` Prometheus scrape job and
   the `transform/oksg-source` processor.
4. **Agent** scrapes `istio-proxy:15020/stats/prometheus`. The relabel rule
   stamps `oksg_source=mesh.istio`. The agent forwards OTLP to the gateway,
   the gateway to the backend.
5. **Backend OTLPNormalizer** runs. The Istio dialect rewrites
   `istio_requests_total` to `servicegraph.request.count`. The assignment
   filter accepts the sample because `oksg_source` matches the assigned
   source. The unit map applies the duration ms→ns conversion when the
   companion `servicegraph.request.duration` arrives.
6. **MeshMetricTopologyBuilder** parses the same envelope and emits
   `topology.Edge{Relation:"calls", Provenance:["mesh.istio"], ...}` with
   protocol, mTLS principal, and response metadata pulled from
   datapoint attributes.
7. **API** serves the metric as `rate(servicegraph.request.count[1m])` and
   the edge as part of `/api/v1/graph`.

If the same cluster also runs the `servicegraph` connector, the planner
would still omit it. If only traces are available (no mesh), the resolver
falls through to `traces.servicegraph`, the planner enables the connector,
and the same canonical name is served end-to-end. The UI never sees the
difference.

## 7. Adding a New Metric

The contract a new metric must satisfy:

1. Add an entry to `defaultCatalog` with semconv-style name, unit, category,
   scopes, and at least one `SourceRef`.
2. Add an entry to `OTLPSemconvMetrics` for ingest unit/kind, **or** add the
   name to `catalogSourcelessExceptions` if it is purely query-derived.
3. If the producer emits a non-canonical name, add a translation to the
   producer's `Dialect` map.
4. Update `TestDefaultCatalogIncludesRequiredMetrics` so the catalog
   regression suite enforces presence.
5. Update `docs/api-reference.md` if the metric introduces a new query
   shape, and `docs/runtime-flow.md` if it introduces a new derivation rule
   (rate, ratio, histogram unit conversion).

## 8. Adding a New Source

1. Add the source ID constant to `canonical.go`.
2. Add the `LogicalSource` entry to `defaultLogicalSources` in `sources.go`
   with the right `Detect` predicate (the predicate must read the
   `ClusterProfile`, not env vars or live cluster state).
3. If the source emits its own dialect, add a `DialectMap` to `dialect.go`
   and reference it from the `LogicalSource`.
4. Cite each source in the catalog entries that can come from it, with a
   priority value relative to existing producers of the same metric.
5. Wire the source in the planner: which receiver/connector emits it, and
   how the Prometheus / OTLP pipeline tags `oksg_source` for it.
6. Update `docs/discovery-flow.md` (detection rule) and this document
   (registry table).

## 9. Files at a Glance

| File | Role |
|------|------|
| `pkg/core/metrics/canonical.go` | `CanonicalMetric`, `MetricCategory`, `MetricScope`, `SourceRef`, `SelectionPolicy`, source ID constants |
| `pkg/core/metrics/catalog.go` | `defaultCatalog`, `DefaultCatalog()` |
| `pkg/core/metrics/sources.go` | `LogicalSource`, `defaultLogicalSources`, `DefaultLogicalSources()`, `Detect` predicates |
| `pkg/core/metrics/dialect.go` | `Dialect`, `DialectMap`, `DialectRegistry`, `PrometheusDialect`, `SpanmetricsDialect`, `ServicegraphDialect`, `IstioDialect` |
| `pkg/core/metrics/resolver.go` | `Resolve`, `Assignment`, `SelectedSources` |
| `pkg/core/metrics/otel_mappings.go` | `OTLPSemconvMetrics`, ingest-side units and kinds |
| `pkg/core/metrics/unit.go` | Unit constants, conversion factors |
| `pkg/core/semconv/keys.go` | OTLP attribute key constants |
| `pkg/backend/ingest/normalizer.go` | `OTLPNormalizer`, `WithAssignment`, source-mismatch drop |
| `pkg/backend/ingest/mesh_metric_topology_builder.go` | Mesh `calls` edges from servicegraph-shaped Istio metrics |
| `pkg/adapters/otel/source_planner.go` | Receivers/connectors gated on `SelectedSources`, `transform/oksg-source` processor |
