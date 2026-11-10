# Metrics Pipeline

This document explains how metric signals travel from heterogeneous producers
to a single canonical, semconv-named series. Four boundaries work together:

1. **Catalog** — every canonical metric the system knows how to render
2. **Source registry** — the logical producers that can emit a canonical metric
3. **Source parsers** — source-owned translation from producer wire format to canonical
4. **Resolver** — picks one winning source per metric for a given cluster

The catalog and resolver live in `pkg/core/metrics`; the static source registry
and parsers live in `pkg/sources`. Their contracts are pure and unit-tested
without a running cluster.

## 1. Canonical Catalog

`pkg/core/metrics/catalog.go` declares every metric the platform exposes
through the API and the UI. `DefaultCatalog()` returns a copy of the in-process
registry; the running list lives in `defaultCatalog`.

Each entry is a `CanonicalMetric` (`pkg/core/metrics/canonical.go`):

```go
type CanonicalMetric struct {
    Name            string          // semconv-style name, e.g. "k8s.pod.cpu.usage"
    DisplayName     string          // operator-facing label
    Unit            Unit            // API-facing unit (cores, bytes, ns, ...)
    Instrument      MetricInstrument // internal gauge, counter, or histogram semantics
    StoredUnit      Unit            // provider-neutral base unit retained by storage
    Category        MetricCategory  // resource | traffic | latency | error | capacity | health
    Scopes          []MetricScope   // entity, edge, or both
    AppliesTo       []EntityKind    // entity kinds valid for entity-scope queries
    Description     string
    Sources         []SourceRef     // {ID, Priority} — which logical sources can produce it
}
```

Important properties:

- **Names are semconv-first.** Producer-specific names (`istio_requests_total`,
  `traces_service_graph_request_total`) never reach storage or the API; their
  stamped source parser translates them before normalization.
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
- **Catalog ↔ ingest invariant.** Every cataloged metric declares its
  provider-neutral instrument and stored base unit. A source parser may emit
  it only when its stamped source owns a supported raw-name path.
- **Catalog inclusion requires a real path.** Add a metric only after its
  receiver, source parser, catalog row, and supported behavior
  test exist. Do not use absence tests to preserve deferred scope.

The current categories are `resource`, `traffic`, `latency`, `error`,
`capacity`, and `health`. Categories drive the Metrics Explorer tab layout.

## 2. Logical Source Registry

`pkg/sources` declares the single compiled module inventory. The capability
profile records deterministic availability for all seven modules, while the
catalog only declares which source IDs can produce each canonical metric.

| ID | Availability evidence | Parser |
|----|-----------------------|--------|
| `kubernetes.structure` | successfully listed Kubernetes discovery inventory | non-metric structural assembly |
| `kubelet`, `host-os`, `kube-cluster` | successfully listed Kubernetes discovery inventory | source-owned parser |
| `prometheus.app` | bounded annotated Service targets | source-owned parser |
| `mesh.istio` | complete injected sidecar mesh evidence | source-owned parser |
| `traces.servicegraph` | derived trace-capable OTLP evidence | source-owned parser |

Each static module owns its source-specific capability detector, collector
contribution, stamps, and parser. The Kubernetes adapter lists the typed
inventory once; no module performs a parallel API probe. The generic resolver
receives only module IDs and capability availability, then writes the selected
IDs and metric assignments to the mounted source plan. Every telemetry-capable
module owns a parser; the structural module intentionally has none. The
kubeletstats receiver emits a mix of `k8s.*` and upstream `container.*` names,
so its source parser keeps the direct `container.*` usage names. CPU
requests/limits from `k8s_cluster` remain under `k8s.container.*`.

The `kube-cluster` source is the metric-native Kubernetes API path for the
focused catalog: container CPU/memory requests and limits, container readiness
and restarts, pod phases and status reasons, and node Ready, MemoryPressure,
and DiskPressure conditions. Structural discovery can still materialize the
broader Kubernetes entity catalog independently of metric collection. Object
snapshots/events from the OTel `k8s_objects` receiver are intentionally out of
scope because that receiver emits logs, not metric samples.

The `prometheus.app` source generates 15-second Service-DNS scrape jobs. This
explicit interval is part of the source contract: the API's exact one-minute
increase and histogram-mean queries need at least two cumulative samples
inside the window. Prometheus otherwise defaults to a one-minute scrape
interval; see the [upstream configuration reference](https://prometheus.io/docs/prometheus/latest/configuration/configuration/).

The pinned Collector Contrib v0.156.0 [kubeletstats contract](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.156.0/receiver/kubeletstatsreceiver/documentation.md) emits CPU usage as a `{cpu}` gauge and volume measurements under the raw `k8s.volume.*` names. Karthos stores CPU values as cores and maps raw volume names to the direct `k8s.pod.volume.*` convention. The [k8s_cluster contract](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.156.0/receiver/k8sclusterreceiver/documentation.md) likewise emits container CPU request/limit in `{cpu}`. The [servicegraph connector contract](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.156.0/connector/servicegraphconnector/README.md) measures duration histograms in seconds; Istio's millisecond histograms are converted to seconds by its source parser. Host CPU/filesystem utilization is stored as a 0..1 ratio. The [system metric convention](https://opentelemetry.io/docs/specs/semconv/system/system-metrics/) defines `system.disk.io` and `system.network.io` as byte counters; Karthos' direction-specific `*.read`/`*.write`/`*.receive`/`*.transmit` rate views remain explicitly derived names because they report bytes per second, not the counter's unit.
The servicegraph connector reads its configured dimensions from span attributes. The gateway copies `k8s.namespace.name` from each trace resource to its spans before the connector, which emits separate client and server namespace labels. The source module uses those labels for calls identity and drops evidence when either side lacks an explicit namespace.

Host OS network dimensions use the public labels `direction` and
`network_interface_name`. Volume dimensions use `volume_name`, `volume_type`,
and optional `persistentvolumeclaim_name`; standard OTLP resource attributes
and supported datapoint attributes normalize to those names. PVC-backed
samples attach to PVC entities, while non-PVC volumes remain attached to their
Pod. All `k8s.pod.volume.*` catalog metrics therefore apply to both PVC and Pod.

The storage request/limit families are intentionally absent from the focused
catalog. `k8s.pod.volume.*` remains the retained mounted-volume capacity surface.

`k8s_cluster` container and pod metrics resolve to the corresponding entities
already materialized by structural discovery. The broader structural catalog
remains available for topology queries even when it has no metric family in
the focused catalog.

Source IDs are exported as constants so the catalog, parser registry, and planner
share a single contract:

```go
SourceKubelet, SourceHostOS, SourceKubeCluster,
SourcePrometheusApp, SourceMeshIstio,
SourceTracesServicegraph
```

## 3. Source Parsing

Each telemetry-capable static module exposes one parser:

```go
Parse(rawMetricName string, attributes map[string]string) (contract.ParsedMetric, bool)
```

The parser is selected only by the datapoint's `karthos_source` / `karthos.source`
stamp. It recognizes its own raw names, expands source facts such as host
disk/network direction and Kubernetes node conditions, and returns the final
canonical name plus any raw-unit conversion. It rejects names outside that
source; there is no cross-source fallback or centralized raw-name registry.

| Source parser | Example result |
|---------------|----------------|
| `prometheusapp.Parse` | `http_requests_total` → `http.server.request.count` |
| `kubelet.Parse` | `container.cpu.usage` → `container.cpu.usage` |
| `kubecluster.Parse` | `k8s.container.cpu_request` → `k8s.container.cpu.request` |
| `hostos.Parse` | `system.disk.io` + `direction=write` → `system.disk.io.write` |
| `servicegraph.Parse`, `istio.Parse` | source wire names → `servicegraph.*` |

Generic ingest and metric-derived calls topology share that parser result,
then validate catalog membership and the mounted source assignment. Source
normalizers may retain bounded labels, resolve entities, and add supported
derived output; they cannot rename the accepted canonical metric.

`hostos.Parse` preserves the dimensions needed to keep the focused host series
truthful: CPU `state`, disk `device`/`direction`, network
`network_interface_name`/`direction`, and filesystem device/mount/type/mode/state.
Disk I/O and network I/O are cumulative counters and use shared explicit
aggregation modes. Paging, load, and connection metrics are outside the focused
host catalog and are neither collected nor parsed.

The Istio parser treats all six protocol metrics as genuine cumulative sums:
gRPC request/response messages, TCP opened/closed connections, and TCP sent/received
bytes. It preserves edge identity, protocol/source dimensions, and base/rate units;
the dedicated mesh job scrapes every 15 seconds so a one-minute aggregation window
reliably contains multiple cumulative samples. The cluster accuracy scenario proves
their raw-to-API behavior with deterministic traffic before audit rows may move from
`no_data`.

## 4. Resolver and Arbitration

When more than one available source declares the same canonical metric (for
example Istio mesh + servicegraph connector both produce
`servicegraph.request.count`), exactly one source must win or the metric
will double-count.

`pkg/core/metrics/resolver.go`:

```go
func Resolve(catalog []CanonicalMetric, sourceIDs []string,
             available func(string) bool) (Assignment, SelectedSources, error)
```

- `Assignment` is `map[canonicalName]sourceID` — one winner per metric.
- `SelectedSources` is the set of source IDs that won at least one metric.
- The available source with the lowest `Priority` value wins.
- A metric with no eligible source is dropped from the assignment (best-effort
  missing data, not an error).

The output drives two consumers:

### 4a. Collection composition and OTel rendering

`pkg/sources` resolves the immutable source plan. `pkg/collector` composes
typed, provider-neutral requirements from the selected modules. The OTel
adapter renders the resulting receivers and connectors. When an active sidecar
Istio source wins all `servicegraph.*` metrics, the generic plan keeps trace
intake requested by the available servicegraph module but omits the derived
servicegraph requirement, so the OTel adapter omits the `service_graph`
connector. The adapter installs `transform/karthos-source` only on
collector-owned receiver/connector pipelines, where it stamps `karthos_source`
from the receiver-determined wire-name namespace. OTLP metrics use separate
`metrics/otlp` pipelines without inferred stamping: already-stamped agent
metrics pass through, while direct external producers must supply a registered
source or the backend rejects them. Application
Prometheus and Istio scrape jobs also stamp `karthos_source` at scrape time via
`relabel_configs`, using the Prometheus-safe `karthos_source` label that the
backend later normalizes to the `source` attribute. Application Prometheus jobs
run on the singleton gateway, require an annotation naming a declared Service
port, use static `<service>.<namespace>.svc.cluster.local:<service-port>` targets, and
preserve `service`, `namespace`, `metrics_path`, and `service_port`. Kubernetes
performs Service-to-`targetPort` translation; these jobs make no per-pod
identity claim. The dedicated Istio job retains pod discovery, but keeps only
pods whose `__meta_kubernetes_pod_node_name` equals `${env:K8S_NODE_NAME}`;
each DaemonSet agent therefore owns the proxies on its own node.

**POC service-graph policy.** A complete injected Istio mesh selects only
`mesh.istio`. A workload Pod has a supported proxy when `istio-proxy` is either
a classic regular container or a Kubernetes native sidecar init container with
`restartPolicy: Always`. A partial mesh is detected when a non-system
controller-owned workload Pod is outside an injected namespace, explicitly sets
`sidecar.istio.io/inject: "false"`, or lacks one of those supported proxy
layouts. Kubernetes, Istio control-plane, and Karthos namespaces are excluded from
that comparison. Ambient mode remains unsupported because it has no per-Pod
proxy scrape contract. Partial meshes globally disable `mesh.istio` and its
scrape job.
When traces are available,
`traces.servicegraph` is the sole service-graph source; without traces, no
service-graph source is selected. This deliberately avoids mixed mesh/trace
coverage in the POC. Namespace-scoped arbitration and boundary peers are not
implemented.

Source-aware POC deployments must use discovery-generated Helm values. A direct
Helm install is unsupported for source-aware telemetry correctness because it
can diverge from the discovery-selected source plan. Generated values are a
point-in-time plan: rerun discovery, plan generation, and deployment whenever
observed workload mesh membership changes. Automatic reconciliation is not part
of this POC.

### 4b. Ingester (`pkg/backend/ingest`)

The shared `MetricSourcePolicy` validates every recognized canonical datapoint
before entity resolution. It requires a registered source stamp and enforces
the exact assigned winner (or a consistently assigned specialization).
Missing, unknown, and mismatched datapoints are dropped while valid siblings in
the acknowledged batch continue. The policy is a pure accepted/dropped
validation decision. Metric calls topology reuses that same decision without
recording it again, preventing invalid edges and double counting.

This is the mechanism that prevents double-counting when the same canonical
metric has two installed producers.

The normalizer preserves source dimensions through a semantic allowlist rather
than arbitrary label passthrough. This keeps the canonical store reusable for
UI, audit, analytics, and future prediction consumers while controlling
cardinality. Prometheus app HTTP metrics preserve Service identity, namespace,
configured Service port, metrics path, and bounded HTTP route/method/status
dimensions, while dropping raw `path` and volatile target `pod` /
`instance` labels. A route is retained only when the application or collector
supplies `http.route` / `http_route`; the normalizer never derives one from a
raw path. Without that bounded route, the canonical application metric remains
a service-level aggregation rather than per-path data. Static Service-DNS
scraping intentionally makes no pod or endpoint-target identity claim.
Servicegraph preserves edge identity, `failed`,
`connection_type`, `virtual_node`, generic `http_method` when emitted, and
distinct `client_http_method` / `server_http_method` dimensions. Labels with
dotted OTLP names are written with Prometheus-safe API keys such as
`http_method`, `http_status_code`, and `span_kind`. Bounded string, boolean,
integer, and floating-point attributes are serialized as Prometheus label
strings; this is required for numeric HTTP status dimensions.

The generated `service_graph` connector instance uses a 15-second metrics flush ticker and a 30-second edge-pairing TTL. Client
and server spans are exported by independent application batch processors, so a
2-second TTL can expire a valid edge before its matching span arrives and
produce incomplete counts or boundary-node series. In collector v0.156,
`connection_type` is absent, `messaging_system`, or `database`; virtual-node
identity is carried separately by the optional `virtual_node=client|server`
label. See the versioned [servicegraph connector contract](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.156.0/connector/servicegraphconnector/README.md).

Counter samples use a physical base unit (`count`, `bytes`, or `seconds`),
never an implied rate. Cumulative Sums retain their running base total; delta
Sums retain the reported interval increment with `temporality=delta`; the
backend keeps no per-series accumulator. Histogram datapoints likewise retain
their reported temporality. Collector v0.156 servicegraph output is cumulative:
its request/error metrics are monotonic cumulative Sums and its latency metrics
are cumulative Histograms. The normalizer therefore preserves the source
temporality rather than inferring it from a metric name or from an
unspecified/gauge payload. See the
versioned [servicegraph connector documentation](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.156.0/connector/servicegraphconnector/README.md).
Query construction is catalog-driven. Public metric reads resolve a
provider-neutral fixed 15-second evaluation grid. `latest` carries the newest
raw observation forward to each evaluation point for at most five minutes,
rather than returning every source sample; each point retains its
cumulative-total or delta-increment temporality. Cumulative `increase`,
`window_rate`, `active_rate`, and histogram means are derived from raw
observations before being placed on that same grid. The final requested `to`
is an evaluation point even when it is not step-aligned. The VictoriaMetrics
adapter preserves precise query bounds, uses a range evaluation for regular
grid points, and adds one exact instant evaluation only for that non-aligned
final point. Its PromQL translation uses an explicit five-minute
`last_over_time` freshness gate, rather than relying on a storage-server
lookbehind default. The adapter owns this translation; it does not define the
public range contract.
Cumulative `increase` and `window_rate` use reset-aware `increase()` only after a
two-sample/history-span maturity gate; delta series use `sum_over_time()`.
`increase` returns the base unit and `window_rate` divides that exact increase
by the full normalized window to return the catalog-declared rate unit.
Cumulative `active_rate` uses `rate()` and delta `active_rate` returns no data.
Histogram means use gated exact increases. VictoriaMetrics is
deployed
with `-dedup.minScrapeInterval=1ms`, so exact OTLP retry replays with identical
labels and timestamps are idempotent at storage. See the
[VictoriaMetrics deduplication contract](https://docs.victoriametrics.com/victoriametrics/index.html#deduplication).
Live metric API range queries set `nocache=1`; otherwise an aligned query result
computed just before a burst can hide newly ingested counter samples until the
VictoriaMetrics response cache expires.

**Convention for status-shaped signals.** Receivers that emit lifecycle state
(e.g. the raw phase metric from `k8s_cluster`) typically encode the enum as an
integer gauge; we keep the integer encoding through the pipeline because
`MetricSample.Value` in this codebase is `float64` only. Do not invent
string-valued samples for new enum metrics.

The names `k8s.pod.volume.inode.{count,free,used}`,
`k8s.container.restart.count`, and `k8s.pod.status.{phase,reason}` adopt
OpenTelemetry vocabulary. Karthos keeps inode counts as gauges, retains its
existing restart counter query modes, and preserves phase and reason as
integer enums with `value_label` at the API boundary. The conventions use
UpDownCounters and represent pod phase and reason as 0/1 series keyed by
attributes. The parsers translate the names emitted by the pinned receivers.
Previously stored samples keep their original names.
[Kubernetes metric conventions](https://opentelemetry.io/docs/specs/semconv/system/k8s-metrics/) ·
[Migration guide](https://opentelemetry.io/docs/specs/semconv/non-normative/k8s-migration/) ·
[Kubelet receiver v0.156.0](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.156.0/receiver/kubeletstatsreceiver/documentation.md) ·
[Cluster receiver v0.156.0](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.156.0/receiver/k8sclusterreceiver/documentation.md)

**Integer-to-string translation at the API boundary.** The integer encoding
is internal: storage, PromQL composition, and the OTLP normalizer all see
integers and never strings. The translation to public plain-text labels
(`healthy`, `running_not_ready`, `restarting`, etc.) lives **only** in
`pkg/core/metrics/health_status.go` and is applied by:

- The provider-neutral metric-store output decorator at `Query` time, which
  sets `MetricSample.ValueLabel` for any health-status metric (see
  `IsHealthStatusMetric`). Storage implementations retain only numeric
  canonical samples; other metrics never carry `ValueLabel`.
- The graph and status handlers, which call the translator to populate
  `enrichedNode.Status` and bucket `health_summary.pods.*` by string key.

Snapshot-wide status and graph enrichment queries do not embed the current entity
IDs in PromQL. The shared health resolver queries the five-minute window once per
health branch and intersects samples with current graph IDs in Go, keeping query
text constant as clusters grow. Single-node lookup retains its exact entity filter.
Responses expose `complete`, `partial`, or `unavailable` quality metadata; a failed
branch degrades `/status` but remains an HTTP 200 enrichment result.

`k8s.pod.status.phase` uses the same boundary with `1=pending`, `2=running`,
`3=succeeded`, `4=failed`, and `5=unknown`. Its readable phase is separate
from derived graph health: a Running phase can contribute to the `healthy`
health-summary bucket without changing the phase value. `k8s.pod.status.reason`
uses the same boundary with `1=evicted`,
`2=node_affinity`, `3=node_lost`, `4=shutdown`,
`5=unexpected_admission_error`, and `6=unknown`. Frontends never render the
integer for these metrics. New consumers (UI, exporters, dashboards) read the
string. The catalog row's `Description` documents both the integer encoding
(for transparency / debugging) and the string labels.

**Composition contract.** The `buildPromQL` switch arm
(`pkg/adapters/storage/victoriametrics/metric_store.go`) is the single source
of the composition. Each enum clause uses `(predicate) * 0 + N` so the
clause emits a sample with value `N` only when its predicate matches and emits
no sample otherwise; PromQL `or` then keeps the first matching clause per
series in priority order. When a clause matches no series the metric returns
empty, which the API serves as "no recent sample" — equivalent to
`unknown`.

## 5. Canonical Storage Semantics

The central catalog declares each metric's provider-neutral `Instrument`
(`gauge`, `counter`, or `histogram`) and `StoredUnit`. Source parsers own
raw-unit conversion. Generic ingest uses the catalog semantics to select
scalar or histogram handling, while memory and VictoriaMetrics use them for
counter rate/increase behavior. API-facing output remains unchanged.

`pkg/core/semconv/keys.go` centralizes the OTLP attribute keys used across
ingest and topology builders (`k8s.pod.name`, `peer.service`,
`k8s.persistentvolumeclaim.name`, mesh-specific keys, the `karthos.source` and
`karthos_source` attribute names, etc.). Updating an attribute key happens in
one place and any code that imports `pkg/core/semconv` benefits.

## 6. End-to-End Example: `servicegraph.request.count`

This metric is the canonical "request rate between two services or workloads"
counter. It can be produced by two sources:

- `mesh.istio` (priority 1) — sidecar `istio_requests_total`
- `traces.servicegraph` (priority 2) — OTel `traces_service_graph_request_total`

Pipeline for a complete active sidecar Istio mesh:

1. **Discovery** records a bounded `mesh.istio` capability only after the
   direct Kubernetes probes confirm a complete injected sidecar mesh.
2. **Resolver** consumes the explicit `available` capability records for
   `mesh.istio` and `traces.servicegraph`; the lower-priority `mesh.istio` source
   wins. `SelectedSources` contains only sources that win at least one
   metric, so `traces.servicegraph` is absent if Istio wins all of its metrics;
   `Assignment["servicegraph.request.count"] = "mesh.istio"`.
3. **Planner** does **not** wire the `service_graph` connector because
   `traces.servicegraph` is absent from `SelectedSources`. It configures the
   `mesh-istio` Prometheus scrape job and
   the `transform/karthos-source` processor.
4. **Agent** scrapes `istio-proxy:15020/stats/prometheus` only when the target
   Pod is on that agent's `K8S_NODE_NAME`. The relabel rule stamps
   `karthos_source=mesh.istio`. The agent forwards OTLP to the gateway, the
   gateway to the backend.
5. **Backend OTLPNormalizer** runs. The Istio parser rewrites
   `istio_requests_total` to `servicegraph.request.count`. The assignment
   filter accepts the sample because `karthos_source` matches the assigned
   source. The unit map applies the duration ms→ns conversion when the
   companion `servicegraph.request.duration` arrives.
6. **MetricCallsTopologyBuilder** parses the same envelope after source-policy
   validation. The static source registry delegates recognition and bounded
   metadata to the validated source module; the generic builder emits the
   resulting `topology.Edge{Relation:"calls", ...}` and deduplicates it.
7. **API** serves the metric using its catalog default (`window_rate`, 1 minute)
   and returns the effective mode/window plus evaluated `from`, `to`, and `step`
   metadata; callers can explicitly select
   `latest`, `increase`, or cumulative `active_rate`. It serves
   the edge as part of `/api/v1/graph`.

If the same complete mesh cluster also runs the `service_graph` connector
instance, the planner still omits it. If only traces are available, Istio has
no injected workload namespace, or discovery marks the mesh partial, the
resolver falls through to `traces.servicegraph`; the planner enables the
connector and does not configure an Istio scrape. The UI never sees two
sources for the same POC service-graph metric.

## 7. Adding a New Metric: End-to-End Checklist

A new canonical metric must pass through six independent contract steps
before a UI tile lights up. **Every one of them is silently failable** —
the unit-test suite catches catalog/parser drift but cannot catch
receiver-config or OTTL-tag mistakes. Both classes of bug have shipped
to production before. Treat this as a sequence, not a punch list, and
finish with live-cluster verification on every PR.

### Step 1 — Receiver-side: confirm the metric is actually produced

For each upstream OTel receiver (kubeletstats, hostmetrics, k8s_cluster,
prometheus, servicegraph connector, istio scrape):

- Open the receiver's `metadata.yaml` at the contrib version pinned in
  `deploy/helm/karthos/values.yaml` (search the file for `tag:`) and
  confirm the metric appears under `metrics:` with `enabled: true`.
- If `enabled: false` (common — e.g. `system.cpu.utilization` in the
  hostmetrics cpu scraper), add the semantic requirement in its source
  module's `contribution.go`, then render it in
  `pkg/adapters/otel/compiler.go`. Add an adapter assertion that the enabled
  metric appears in the rendered configuration.
- Note the metric's **wire-format name** exactly as written in the
  upstream `metadata.yaml`. This is what the OTel pipeline sees on the
  wire, and it often differs from the OTel semconv canonical we use in
  the catalog. Examples: kubeletstats emits `container.cpu.usage`
  (no `k8s.` prefix); k8s_cluster emits `k8s.container.cpu_request`
  (underscore segment, not dotted).

### Step 2 — Source parser: translate wire name → catalog canonical

If the wire name and the catalog canonical differ:

- Add or update the matching module's private raw-name map and `Parse`
  behavior in `pkg/sources/<source>/parser.go`. The parser may also use
  attributes to expand one wire metric into a canonical directional or
  condition-specific name.
- Add a positive parser behavior test in that source package and a
  round-trip normalizer test that feeds the stamped raw OTLP name and asserts
  the canonical sample.

### Step 3 — OTTL source tagger: stamp `karthos_source` correctly

`sourceTagger` in `pkg/adapters/otel/compiler.go` renders the generic source
stamp selectors on
collector-owned agent and gateway pipelines **before** source parsing (which
lives in the backend). It never runs on `metrics/otlp`. OTTL rules therefore see
**wire-format names**, not canonical names.
Two consequences:

- Rules that match against canonical names that only exist post-parse
  are dead code. `^k8s\.container\.` at the gateway only matches
  k8s_cluster output (kubeletstats container metrics arrive as raw
  `container.*`, not `k8s.container.*`).
- The first matching rule wins (`datapoint.attributes["karthos_source"] == nil`
  guard). Rule ordering matters when prefixes overlap. Place specific
  rules before broad ones.

For each metric:

- Identify which receiver/connector emits it on the wire and confirm
  exactly one tagger rule matches the wire name.
- Confirm the source ID stamped by that rule equals the `Sources[].ID`
  in the catalog row. Mismatch is silently fatal:
  `MetricSourcePolicy.Validate` (`pkg/backend/ingest/source_policy.go`) drops every
  datapoint whose tag disagrees with the catalog assignment.
- Add an OTel adapter test asserting the rendered tagger statement is present.

### Step 4 — Catalog: advertise the metric

Add a `defaultCatalog` row in `pkg/core/metrics/catalog.go`:

- `Name`: post-parser canonical name.
- `Unit`, `Instrument`, and `StoredUnit`: API unit and provider-neutral
  storage behavior. Counters use the accumulated base `StoredUnit`; source
  parser conversion turns raw producer values into that unit.
- `Category`: `resource` / `traffic` / `latency` / `error` / `capacity`
  / `health`.
- `Scopes`: `entity`, `edge`, or both.
- `Sources`: the source IDs that can produce this metric, with priority.
  The IDs **must** match what the OTTL tagger stamps in step 3.

Update `TestDefaultCatalogIncludesRequiredMetrics` to enforce presence plus
unit/category/sources, then add the source-module and normalizer behavior
coverage for the supported path.

For status/lifecycle signals: keep the integer encoding all the way through
the pipeline. `MetricSample.Value` is `float64`; do not invent string-valued
samples. The receiver's own integer encoding (e.g. phase 1–5) is
the source of truth; map it to a public enum in `buildPromQL` if needed.

### Step 6 — Live-cluster verification (mandatory)

Unit tests do not exercise the receiver pipeline, so receiver-config
and OTTL-tag drift can ship green tests but produce zero data. After
merging, redeploy to kind and verify both layers:

- VictoriaMetrics has the Prometheus-translated name. Port-forward
  is not needed; exec into the VM pod:
  ```
  kubectl exec -n karthos-system karthos-victoria-metrics-single-server-0 -- \
    wget -qO- 'http://127.0.0.1:8428/api/v1/label/__name__/values' \
    | python3 -c "import json,sys; print('<your_metric_underscore_form>' in json.load(sys.stdin)['data'])"
  ```
- Backend API serves the metric with the expected source label:
  ```
  curl 'http://localhost:18081/api/v1/metrics?metric_name=<canonical>&entity_id=<id>' \
    | jq '.data | {samples: (.samples | length), source: .samples[-1].labels.source}'
  ```
- Frontend Metrics Explorer applies catalog applicability to the selected entity.
  Its focused Container, Node, and Pod projections preserve dimensional truth:
  label-distinct series remain separately inspectable rather than being chosen or
  silently combined for a presentation summary.

### Forensic toolkit when a metric is missing

If a metric is advertised but missing in storage, the drop is at one of
four points. Bisect with these reads:

1. **Receiver accepting?** Port-forward the gateway's `:8888/metrics`
   endpoint and read
   `otelcol_receiver_accepted_metric_points{receiver="..."}`. Zero
   means the receiver isn't producing — go back to Step 1.
2. **Reaching backend?** Compare with
   `otelcol_exporter_sent_metric_points{exporter="otlp_grpc"}` on the
   gateway. Equal means everything left.
3. **Dropped at normalizer?** Inspect the source stamp and mounted source plan;
   supported tests assert accepted and dropped canonical samples directly.
4. **Source parser hit?** Add a normalizer test that feeds the
   wire name and asserts the canonical output. If it fails, the source parser
   or source stamp is wrong.

### Misc follow-ups when the metric introduces new query shape

- Update `docs/api-reference.md` if the metric introduces a new query
  shape (e.g. histogram quantile, ratio derivation).
- Update `docs/runtime-flow.md` if it introduces a new ingest-time
  derivation rule (rate, ratio, histogram unit conversion).

### Pod lifecycle topology boundary

`k8s.pod.status.phase` is a metric signal; it is not used as a missing-series
heuristic. The Kubernetes lifecycle adapter observes `Succeeded` and `Failed`
Pods and, after a fixed 60-second grace, retires only the exact UID/node
`runs_on` assignment. Pod and Node endpoints remain name-based, while the
`runs_on` edge identity includes the Pod UID; a replacement can therefore be
active while the old assignment remains auditable. The graph store records the
RFC3339 `retired_at` value in edge metadata without changing `first_seen` or
final `last_seen`. Latest-graph and edge queries exclude that `runs_on` edge;
retirement state remains internal lifecycle-reconciliation data. `AllowsRunsOn` denies refresh synchronously at
the recorded deadline even if the asynchronous persistence callback is delayed.
At backend startup, persisted active UID-versioned assignments are reconciled
against the first authoritative Pod List/Watch state before metric refreshes
are accepted, retiring any old UID that is absent from that state. A terminal
UID with a confirmed initial marker keeps its full grace period even if it is
deleted or replaced before the deadline; non-terminal deletions and
replacements retire immediately. Retirement
persistence retries until backend shutdown with a bounded per-attempt backoff.
Terminal observation synchronously stores one immutable `terminal_deadline`
with the cluster and Pod identity before allowing grace-period refreshes. If
that initial write fails, the terminal UID is fail-closed immediately; its
structural refreshes may be lost while the original deadline is retried
asynchronously without being extended. A retry-only marker is explicitly
unconfirmed: if its UID is absent or replaced at restart, reconciliation
retires it immediately rather than preserve grace for a stale assignment. A
restart preserves grace for a confirmed initial marker even when the old Pod no
longer appears in the List/Watch result, or when that same terminal UID is
still listed: reconciliation returns the existing deadline to the tracker and
never resets it. Reconciliation is restricted to matching
`k8s.cluster.name`. At the deadline, latest-graph reads exclude the edge
and the subsequent retirement marker provides the audit timestamp.
Storage enforces the one-way UID assignment transition from active to pending
terminal to retired. A delayed terminal-marker retry receives the retired state
after a deletion/replacement has won and must not recreate pending grace.
The retirement marker applies only to `runs_on`, not to another relation that
happens to carry a metadata key of the same name. `graph?at=` selects a
retained materialized snapshot from the public seven-day window; it does not
reconstruct arbitrary historical graph state or provide unbounded history.

## 8. Adding a New Source

1. Add one static module in `pkg/sources` with a pure detector over the typed
   Kubernetes inventory, semantic collection requirement, source stamps, and parser.
2. Register its deterministic capability record in the static source registry;
   do not add a runtime override, fallback, or central discovery switch.
3. Cite the source ID in catalog entries that can come from it, with a
   priority value relative to existing producers of the same metric.
4. Add unit and contract tests for the module and its source-plan assignment.
5. Update `docs/discovery-flow.md` (detection rule) and this document
   (registry table).

## 9. Files at a Glance

| File | Role |
|------|------|
| `pkg/core/metrics/canonical.go` | `CanonicalMetric`, `MetricCategory`, `MetricScope`, `SourceRef`, source ID constants |
| `pkg/core/metrics/catalog.go` | `defaultCatalog`, `DefaultCatalog()` |
| `pkg/sources/registry.go` | Compiled source IDs, module inventory, and detector dispatch |
| `pkg/sources/detection` | Typed Kubernetes discovery inventory shared by static detectors |
| `pkg/sources/{kubelet,hostos,kubecluster,prometheusapp,istio,servicegraph}/parser.go` | Source-owned stamped raw-name parsing and conversion |
| `pkg/core/metrics/resolver.go` | `Resolve`, `Assignment`, `SelectedSources` |
| `pkg/core/metrics/unit.go` | Unit constants, conversion factors |
| `pkg/core/semconv/keys.go` | OTLP attribute key constants |
| `pkg/backend/ingest/normalizer.go` | Generic OTLP iteration, `OTLPNormalizer`, shared source-policy enforcement, conversion application, and raw sample emission |
| `pkg/backend/ingest/source_policy.go` | Registered provenance and assignment validation |
| `pkg/sources/contract` | Narrow normalization plan, resolver interfaces, and bounded-label/entity helpers |
| `pkg/sources/contract/source_plan.go` | Mounted resolved source-plan contract and static inventory validation |
| `pkg/collector/spec` | Provider-neutral collection requirements and `CollectionPlan` |
| `pkg/collector/composer.go` | Deterministic generic composition from selected modules and the resolved source plan |
| `pkg/sources/{kubelet,hostos,kubecluster,prometheusapp,servicegraph,istio}` | Source-owned normalization for kubelet, host OS, kube cluster, Prometheus app, servicegraph, and Istio |
| `pkg/sources/{kubelet,hostos,kubecluster,prometheusapp,servicegraph,istio}/contribution.go` | Source-owned semantic collection requirements |
| `pkg/backend/ingest/metric_calls_topology_builder.go` | Generic metric-derived `calls` edge assembly after source-module classification |
| `pkg/adapters/otel/compiler.go` | Renders OpenTelemetry-specific configuration from the generic collection plan |

## Query-time error interpretation

Status-partitioned `servicegraph.request.count` series are authoritative. Istio normalization preserves HTTP/gRPC outcome dimensions and no longer materializes an additional error counter. Its statically registered query view exposes the existing `servicegraph.request.error.count` name using original request series: nonzero gRPC status is a failed RPC; gRPC zero is classified success even if an HTTP status differs; non-gRPC HTTP500–599 is a server failure and HTTP100–499 is classified nonfailure. A missing gRPC status on gRPC traffic is unknown, not zero. No application root cause is inferred.

Source modules own `contract.QueryView` predicates and derivation metadata. The VictoriaMetrics adapter renders predicates before counter rollups, keeps original dimensions and reset/freshness semantics, and emits zero only for fresh classified nonfailure partitions. It ignores historical stored Istio error counters; no historical data is rewritten. The trace servicegraph source retains its connector-native failed counter, whose meaning is client OR server span status ERROR, not the Istio status policy. Responses carry optional `derivation` metadata (id, version, input_metric, interpretation), never additional stored labels. Catalog coverage/freshness for derived views follows classified input series.

The exact deployed contracts were checked against Istio1.28.2, collector-contrib0.156.0 (evaluation relay0.155.0), and VictoriaMetrics1.138.0: [Istio1.28 metrics](https://raw.githubusercontent.com/istio/istio.io/release-1.28/content/en/docs/reference/config/metrics/index.md), [servicegraph connector0.156 source](https://github.com/open-telemetry/opentelemetry-collector-contrib/blob/v0.156.0/connector/servicegraphconnector/connector.go), [MetricsQL1.138 implementation](https://github.com/VictoriaMetrics/VictoriaMetrics/blob/v1.138.0/app/vmselect/promql/rollup.go). Mesh scrapes every15s; kubelet collection every30s. MetricsQL increase includes the pre-window sample and its own reset handling; independently bracketing source totals are not an exact API-query oracle.
