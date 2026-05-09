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
    AppliesTo       []EntityKind    // entity kinds valid for entity-scope queries
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
- **AppliesTo is authoritative for entity-kind applicability.** `applies_to`
  answers "which graph entity kinds can this metric be queried against?" and is
  distinct from `Sources`, which answers "which collector or tool can produce
  this metric?" The frontend must not infer entity applicability from source IDs.
- **Sources drive arbitration**, not detection. A metric with several entries
  in `Sources` declares more than one possible producer; the resolver picks
  exactly one per cluster.
- **Query-derived metrics have no Sources.** `servicegraph.request.error.ratio`,
  `oksg.pod.health.status`, and `oksg.container.health.status` are composed at
  query time from other catalog metrics — see the *Query-derived enum metrics*
  section below and the `buildPromQL` switch arms in
  `pkg/adapters/storage/victoriametrics/metric_store.go`. They are listed in
  `catalogSourcelessExceptions` (`pkg/core/metrics/catalog_test.go`).
- **Catalog ↔ ingest invariant.** Every cataloged metric (other than the
  query-derived exceptions tracked in `catalogSourcelessExceptions`) must
  appear in `OTLPSemconvMetrics` (`pkg/core/metrics/otel_mappings.go`); the
  test `TestDefaultCatalogMetricsHaveIngestOrQuerySupport` enforces this.
- **Unsupported metrics stay out of the catalog.** Names that have no real
  ingest path (e.g. `k8s.pod.network.io.*`, `k8s.node.*` capacity gauges) are
  explicitly excluded by `TestDefaultCatalogExcludesUnsupportedMetrics` so
  the UI never advertises a metric the backend cannot produce.

The current categories are `resource`, `traffic`, `latency`, `error`,
`capacity`, and `health`. Categories drive the Metrics Explorer tab layout.

## 2. Logical Source Registry

`pkg/core/metrics/sources.go` declares the producers OKSG knows how to talk
to. `DefaultLogicalSources()` returns the built-in registry:

| ID | DisplayName | Detect | Dialect |
|----|-------------|--------|---------|
| `kubelet` | Kubelet stats | always | `KubeletDialect` |
| `host-os` | Host OS metrics | always | none |
| `kube-cluster` | Kubernetes API | always | `KubeClusterDialect` |
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
| `KubeClusterDialect` | `k8s.container.cpu_request` → `k8s.container.cpu.request`, `k8s.container.memory_limit` → `k8s.container.memory.limit`, plus the cpu_limit and memory_request equivalents |
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

### 4c. Query-derived enum metrics

A small number of catalog metrics are **composed at query time** from other
catalog metrics rather than ingested directly. They are listed in
`catalogSourcelessExceptions` (`pkg/core/metrics/catalog_test.go`) and have an
empty `Sources` list. Today three such metrics exist:

- `servicegraph.request.error.ratio` — `error.count / request.count`.
- `oksg.container.health.status` — integer-encoded enum derived per container
  from `k8s.container.ready` and `rate(k8s.container.restarts[5m])`. Public
  values: `1=healthy`, `2=running_not_ready`, `3=restarting`. Priority order
  enforced by PromQL `or` clause ordering: restarting > not_ready > healthy.
- `oksg.pod.health.status` — integer-encoded enum derived per pod from
  `k8s.pod.phase`. Public values: `1=healthy` (Running), `4=pending`,
  `5=succeeded`, `6=failed`, `7=unknown`. Pod-scope rollups of
  `running_not_ready` (2) and `restarting` (3) require a `pod_id` label on
  container metrics — those states are observable today only at container
  scope. Tracked as a follow-up plan.

**Convention for status-shaped signals.** Receivers that emit lifecycle state
(e.g. `k8s.pod.phase` from `k8s_cluster`) typically encode the enum as an
integer gauge; we keep the integer encoding through the pipeline because
`MetricSample.Value` in this codebase is `float64` only. Do not invent
string-valued samples for new enum metrics.

**Integer-to-string translation at the API boundary.** The integer encoding
is internal: storage, PromQL composition, and the OTLP normalizer all see
integers and never strings. The translation to public plain-text labels
(`healthy`, `running_not_ready`, `restarting`, etc.) lives **only** in
`pkg/core/metrics/health_status.go` and is applied by:

- The metric store at `Query` time, which sets `MetricSample.ValueLabel` for
  any health-status metric (see `IsHealthStatusMetric` and the in-memory and
  VictoriaMetrics adapters). Other metrics never carry `ValueLabel`.
- The graph and status handlers, which call the translator to populate
  `enrichedNode.Status` and bucket `health_summary.pods.*` by string key.

Frontends never see the integer for these metrics. New consumers (UI, exporters,
dashboards) read the string. The catalog row's `Description` documents both the
integer encoding (for transparency / debugging) and the string labels.

**Composition contract.** The `buildPromQL` switch arm
(`pkg/adapters/storage/victoriametrics/metric_store.go`) is the single source
of the composition. Each enum clause uses `(predicate) * 0 + N` so the
clause emits a sample with value `N` only when its predicate matches and emits
no sample otherwise; PromQL `or` then keeps the first matching clause per
series in priority order. When a clause matches no series the metric returns
empty, which the API serves as "no recent sample" — equivalent to
`unknown`.

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

## 7. Adding a New Metric: End-to-End Checklist

A new canonical metric must pass through six independent contract steps
before a UI tile lights up. **Every one of them is silently failable** —
the unit-test suite catches catalog/mapping/dialect drift but cannot catch
receiver-config or OTTL-tag mistakes. Both classes of bug have shipped
to production before. Treat this as a sequence, not a punch list, and
finish with live-cluster verification on every PR.

### Step 1 — Receiver-side: confirm the metric is actually produced

For each upstream OTel receiver (kubeletstats, hostmetrics, k8s_cluster,
prometheus, spanmetrics connector, servicegraph connector, istio scrape):

- Open the receiver's `metadata.yaml` at the contrib version pinned in
  `deploy/helm/oksg-otel/values.yaml` (search the file for `tag:`) and
  confirm the metric appears under `metrics:` with `enabled: true`.
- If `enabled: false` (common — e.g. `system.cpu.utilization` in the
  hostmetrics cpu scraper), enable it explicitly under the receiver's
  `metrics:` block in `pkg/adapters/otel/source_planner.go`. Add an
  assertion in `pkg/adapters/otel/source_planner_test.go` that the
  enable lives in the generated config.
- Note the metric's **wire-format name** exactly as written in the
  upstream `metadata.yaml`. This is what the OTel pipeline sees on the
  wire, and it often differs from the OTel semconv canonical we use in
  the catalog. Examples: kubeletstats emits `container.cpu.usage`
  (no `k8s.` prefix); k8s_cluster emits `k8s.container.cpu_request`
  (underscore segment, not dotted).

### Step 2 — Dialect: translate wire name → catalog canonical

If the wire name and the catalog canonical differ:

- Add a `DialectMap` entry to the source's dialect in
  `pkg/core/metrics/dialect.go`.
- Confirm `LogicalSource.Dialect` for that source is set in
  `pkg/core/metrics/sources.go`. (A `nil` dialect means the source emits
  canonical-shaped names directly; setting a dialect for the first time
  requires this wire-up.)
- Add a translation assertion in `pkg/core/metrics/dialect_test.go` and
  a round-trip normalizer test in
  `pkg/backend/ingest/normalizer_test.go` that feeds the raw OTLP wire
  name and asserts the dotted canonical comes out (mirror the
  `RawKubelet*` and `RawKubeCluster*` test patterns).

### Step 3 — OTTL source tagger: stamp `oksg_source` correctly

`buildOksgSourceTagger` in `pkg/adapters/otel/source_planner.go` runs at
the agent and gateway **before** the dialect (which lives in the
backend). So OTTL rules see **wire-format names**, not canonical names.
Two consequences:

- Rules that match against canonical names that only exist post-dialect
  are dead code. `^k8s\.container\.` at the gateway only matches
  k8s_cluster output (kubeletstats container metrics arrive as raw
  `container.*`, not `k8s.container.*`).
- The first matching rule wins (`attributes["oksg_source"] == nil`
  guard). Rule ordering matters when prefixes overlap. Place specific
  rules before broad ones.

For each metric:

- Identify which receiver/connector emits it on the wire and confirm
  exactly one tagger rule matches the wire name.
- Confirm the source ID stamped by that rule equals the `Sources[].ID`
  in the catalog row. Mismatch is silently fatal:
  `shouldDropForSource` (`pkg/backend/ingest/normalizer.go`) drops every
  datapoint whose tag disagrees with the catalog assignment.
- Add a planner test asserting the tagger statement is present
  (mirror `TestPlanTagsK8sContainerMetricsAsKubeCluster`).

### Step 4 — OTLP mapping: declare unit/kind/conversion

In `pkg/core/metrics/otel_mappings.go`, add an entry to
`OTLPSemconvMetrics` keyed by the **post-dialect canonical** name:

```go
"k8s.container.cpu.request": {ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge},
```

Set `ConversionFactor` to convert from the producer's unit to the
canonical unit (cores → millicores: 1000.0; seconds → nanoseconds: 1e9;
otherwise 1.0). Set `Kind` to gauge / sum / counter / histogram based on
the upstream `metadata.yaml`. Without this entry the normalizer silently
drops every datapoint at the type-check (`normalizer.go` ingest loop).

### Step 5 — Catalog: advertise the metric

Add a `defaultCatalog` row in `pkg/core/metrics/catalog.go`:

- `Name`: post-dialect canonical, exactly matching `OTLPSemconvMetrics`.
- `Unit`: the canonical unit (matches `OTLPSemconvMetrics`).
- `Category`: `resource` / `traffic` / `latency` / `error` / `capacity`
  / `health`.
- `Scopes`: `entity`, `edge`, or both.
- `Sources`: the source IDs that can produce this metric, with priority.
  The IDs **must** match what the OTTL tagger stamps in step 3.

Update `TestDefaultCatalogIncludesRequiredMetrics` to enforce presence
plus unit/category/sources. Confirm the metric is not in
`TestDefaultCatalogExcludesUnsupportedMetrics`.

If the metric is purely query-derived (e.g. ratios computed in PromQL,
status enums composed from other metrics), the contract is different:
- Omit `Sources` from the catalog row.
- Add the name to `catalogSourcelessExceptions` (`pkg/core/metrics/catalog_test.go`).
- Add a `buildPromQL` switch arm in
  `pkg/adapters/storage/victoriametrics/metric_store.go` **before** adding
  the metric to `required` in `TestDefaultCatalogIncludesRequiredMetrics`,
  otherwise queries return empty silently with green tests.
- Document the composition rule and (for enum-shaped metrics) the
  integer→label mapping in the catalog row's `Description`. The frontend
  reads this via `/api/v1/metrics/catalog` — do not hard-code the mapping
  in UI code.

For status/lifecycle signals: keep the integer encoding all the way through
the pipeline. `MetricSample.Value` is `float64`; do not invent string-valued
samples. The receiver's own integer encoding (e.g. `k8s.pod.phase` 1–5) is
the source of truth; map it to a public enum in `buildPromQL` if needed.

### Step 6 — Live-cluster verification (mandatory)

Unit tests do not exercise the receiver pipeline, so receiver-config
and OTTL-tag drift can ship green tests but produce zero data. After
merging, redeploy to kind and verify both layers:

- VictoriaMetrics has the Prometheus-translated name. Port-forward
  is not needed; exec into the VM pod:
  ```
  kubectl exec -n oksg-system oksg-otel-victoria-metrics-single-server-0 -- \
    wget -qO- 'http://127.0.0.1:8428/api/v1/label/__name__/values' \
    | python3 -c "import json,sys; print('<your_metric_underscore_form>' in json.load(sys.stdin)['data'])"
  ```
- Backend API serves the metric with the expected source label:
  ```
  curl 'http://localhost:18081/api/v1/metrics?metric_name=<canonical>&entity_id=<id>' \
    | jq '.data | {samples: (.samples | length), source: .samples[-1].labels.source}'
  ```
- Frontend Metrics Explorer renders a non-empty chart for the metric on
  a Container/Node/Workload entity in the appropriate category tab.

### Forensic toolkit when a metric is missing

If a metric is advertised but missing in storage, the drop is at one of
four points. Bisect with these reads:

1. **Receiver accepting?** Port-forward the gateway's `:8888/metrics`
   endpoint and read
   `otelcol_receiver_accepted_metric_points{receiver="..."}`. Zero
   means the receiver isn't producing — go back to Step 1.
2. **Reaching backend?** Compare with
   `otelcol_exporter_sent_metric_points{exporter="otlp"}` on the
   gateway. Equal means everything left.
3. **Dropped at normalizer?** Backend logs with normal verbosity do
   not record OTLP-mapping misses. Check
   `OTLPNormalizer.DroppedSampleCount()` (Prom-self-metric or test
   instrumentation) for source-mismatch drops.
4. **Dialect translation hit?** Add a normalizer test that feeds the
   wire name and asserts the canonical output. If it fails, the dialect
   is wrong (or `LogicalSource.Dialect` is still `nil`).

### Misc follow-ups when the metric introduces new query shape

- Update `docs/api-reference.md` if the metric introduces a new query
  shape (e.g. histogram quantile, ratio derivation).
- Update `docs/runtime-flow.md` if it introduces a new ingest-time
  derivation rule (rate, ratio, histogram unit conversion).

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
