# API Reference

REST API served by the backend on port **8080** (configurable via `--api-listen-address` / `KARTHOS_BACKEND_API_LISTEN_ADDRESS`).

For local e2e verification, prefer a dedicated local port-forward such as `18081:8080` so the backend API does not collide with the `gateway-go` traffic port-forward on `18080`.

All endpoints return JSON. Success responses use `{"data": ...}`, errors use `{"error": "...", "code": N}`.
The [OpenAPI description](openapi.yaml) records the current routes and wire shapes.

## Pagination

The list endpoints below support cursor-based pagination in their snapshot or
metric query form. Graph edge observation windows return all matching writes
without a snapshot cursor.

| Param | Type | Default | Description |
|-------|------|---------|-------------|
| `page_size` | int | 100 | Items per page (max 1000) |
| `next_token` | string | -- | Opaque cursor from a previous response |

Paginated responses use a `PaginatedResponse` envelope:

```json
{
  "data": {"items": [...], "total": 250},
  "next_token": "MjAw"
}
```

`next_token` is omitted when there are no more pages. `total` always reflects the full (unpaginated) count.

Applies to: `/graph/nodes`, snapshot `/graph/edges`, `/entities`, `/metrics`,
`/metrics/series`.

## Status

### GET /api/v1/status

Backend health, component availability, and snapshot freshness. This endpoint reflects backend stores, entity registry availability, and graph freshness; it does not report frontend deployment readiness.

```bash
curl -s http://localhost:18081/api/v1/status | jq .
```

Response:

```json
{
  "data": {
    "system_status": "nominal",
    "reasons": [],
    "version": {"major": 1, "minor": 0},
    "components": {
      "metric_store": "available",
      "graph_store": "available",
      "metadata_store": "available",
      "entity_registry": "available"
    },
    "uptime_seconds": 3600,
    "last_snapshot_at": "2026-04-02T15:22:12Z",
    "last_discovery_at": "2026-03-31T01:35:36Z",
    "snapshot_freshness_seconds": 12.5,
    "data_stale": false,
    "health_summary": {
      "status": "complete",
      "nodes": {"status": "complete", "healthy": 3, "unhealthy": 0, "unknown": 0},
      "pods": {"status": "complete", "healthy": 12, "pending": 0, "succeeded": 0, "failed": 0, "unknown": 2}
    }
  }
}
```

`health_summary` is present whenever a graph snapshot exists. Node health uses `k8s.node.condition.ready`; pod buckets derive directly from `k8s.pod.phase`; entities without a recent sample fall through to `unknown`.

Each branch and the aggregate carry a quality value: `complete`, `partial`, or `unavailable`. A successful metric query remains `complete` even when samples are missing. A failed branch becomes `unavailable`, all applicable current entities in that branch become unknown, and `system_status` becomes `degraded` with a generic branch reason. If no MetricStore is configured, the snapshot still produces an `unavailable` summary populated with current graph entity counts. Graph and capability-profile read failures likewise produce a degraded status with a safe reason while component availability remains unchanged. Detailed store errors are logged but not returned, and these enrichment failures keep HTTP 200.

Snapshot-wide health lookups deliberately omit entity-ID filters, so query size stays constant as the graph grows; the API intersects recent results with current graph IDs in Go. The single-node endpoint retains an exact one-entity filter.

The `unknown` bucket can include pod entities that no longer exist in the cluster but have not yet aged out of the entity registry.

Pod bucket keys are `healthy`, `pending`, `succeeded`, `failed`, and `unknown`. Container graph status derives from `k8s.container.ready` plus the five-minute `k8s.container.restarts` rate.

Status values:

| Value | Meaning |
|-------|---------|
| `nominal` | All stores available AND snapshot fresh |
| `degraded` | Stores available but snapshot stale (> `--status-stale-seconds`), a graph or capability-profile read failed, non-critical stores are unavailable, unhealthy nodes are detected, or a health-summary branch query failed |
| `disconnected` | GraphStore or EntityRegistry unavailable |

Configuration:

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--status-stale-seconds` | `KARTHOS_BACKEND_STATUS_STALE_SECONDS` | `360` | Snapshot staleness threshold in seconds |

### GET /api/v1/discovery

Capability profile summary. Returns 404 if no capability profile has been loaded.

```bash
curl -s http://localhost:18081/api/v1/discovery | jq .
```

Response:

```json
{
  "data": {
    "cluster_id": "...",
    "cluster_name": "karthos-test",
    "discovered_at": "2026-03-30T10:00:00Z",
    "capabilities": [
      {
        "source_id": "kubernetes.structure",
        "kind": "direct",
        "status": "available",
        "signals": ["structure"]
      },
      {
        "source_id": "prometheus.app",
        "kind": "direct",
        "status": "unavailable"
      }
    ]
  }
}
```

Unavailable sources may serialize `signals` as `null`; clients should treat
that as an empty signal list.

## Graph

### GET /api/v1/graph

The newest complete materialized topology snapshot with expanded summary.
Every materialization is an immutable point-in-time graph: the direct
Kubernetes structure is authoritative for membership, and only fresh active
telemetry/lifecycle edges whose endpoints still exist in that structure are
included. Materialized snapshots are retained for seven days.

```bash
curl -s http://localhost:18081/api/v1/graph | jq .data.summary
```

Response shape:

```json
{
  "data": {
    "snapshot": {"id": 481, "created_at": "2026-04-02T15:22:12Z"},
    "version": {"major": 1, "minor": 0},
    "from": "2026-04-02T14:52:12Z",
    "to": "2026-04-02T15:22:12Z",
    "observed_at": "2026-04-02T15:22:12Z",
    "created_at": "2026-04-02T15:22:12Z",
    "health_enrichment": {"status": "complete", "pods": "complete", "containers": "complete"},
    "graph": {"timestamp": "...", "nodes": [...], "edges": [...]},
    "summary": {
      "node_count": 47,
      "edge_count": 76,
      "kind_counts": {"pod": 18, "container": 18, "namespace": 8, "node": 1},
      "relation_counts": {"contains": 40, "runs_on": 20, "calls": 2},
      "snapshot_age_seconds": 12.5,
      "available_filters": {
        "kinds": ["container", "namespace", "node", "pod"],
        "namespaces": ["default", "karthos-demo", "karthos-system"],
        "relations": ["calls", "contains", "runs_on"]
      }
    }
  }
}
```

Pass `at=<RFC3339 timestamp>` to select the newest retained materialized
snapshot at or before that instant from the public seven-day retention window.
It does not reconstruct arbitrary historical graph state. The timestamp cannot
be in the future.
`observed_at` is the materialization instant. `from` and `to` bound the
first and last evidence represented by the graph; nodes and edges expose
their own `first_seen`, `last_seen`, and source `provenance` where known.
Returns 400 for an invalid or future `at`, and 404 if no current snapshot (or
no retained snapshot at the requested time) exists.

### GET /api/v1/graph/nodes

List enriched graph nodes with optional filtering and pagination.

| Param | Type | Description |
|-------|------|-------------|
| `kind` | string | Filter by entity kind (`pod`, `service`, `node`, etc.) |
| `namespace` | string | Filter by namespace |
| `at` | RFC3339 timestamp | Select the newest retained snapshot at or before this instant |
| `page_size` | int | Items per page (default 100) |
| `next_token` | string | Opaque pagination cursor bound to the selected snapshot |

```bash
curl -s "http://localhost:18081/api/v1/graph/nodes?kind=pod&page_size=10" | jq .
```

Response:

```json
{
  "data": {
	"snapshot": {"id": 481, "created_at": "2026-04-02T15:22:12Z"},
    "health_enrichment": {"status": "complete", "pods": "complete", "containers": "complete"},
    "nodes": [
      {"id": "...", "kind": "pod", "name": "pod-a", "namespace": "default", "cluster": "karthos-test", "status": "unknown", "labels": {...}}
    ],
    "total": 18
  },
  "next_token": "MTA="
}
```

Nodes are enriched from the entity registry with `name`, `namespace`, `cluster`. Per-node `status` is `healthy | running_not_ready | restarting | pending | succeeded | failed | unknown`. Pod status derives from `k8s.pod.phase`; container status derives from `k8s.container.ready` and the five-minute restart rate at the selected graph instant. Other kinds are `"unknown"`. `health_enrichment` reports aggregate, pod, and container query quality. Aggregate runtime health is exposed through `/api/v1/status.health_summary`.
When no phase metric is available, a Pod uses the phase captured in its
Kubernetes structural snapshot. Failed and Succeeded Pods remain in snapshots
while the Kubernetes API still lists them, then remain queryable in retained
history after deletion.

The `data.snapshot` object identifies the exact graph that produced the page.
Repeat both `at` and `next_token` when paging a historical result. A cursor
from another snapshot is rejected rather than mixing graph versions.

### GET /api/v1/graph/nodes/{id}

Single node with enriched inbound/outbound edges including related entity summaries.
Accepts the same optional `at=<RFC3339 timestamp>` selection as the graph list
endpoints and returns `data.snapshot` for the selected materialized graph.

```bash
curl -s http://localhost:18081/api/v1/graph/nodes/<entity-id> | jq .
```

Response:

```json
{
  "data": {
	"snapshot": {"id": 481, "created_at": "2026-04-02T15:22:12Z"},
    "health_enrichment": {"status": "complete", "pods": "complete", "containers": "complete"},
    "node": {"id": "...", "kind": "service", "name": "svc-a", "namespace": "default", "cluster": "karthos-test", "status": "unknown", "labels": {...}},
    "edges": {
      "outbound": [
        {
          "id": "...", "source": "...", "target": "...", "relation": "calls",
          "confidence": 1.0, "provenance": ["traces.otlp"], "first_seen": "...", "last_seen": "...",
          "related_node": {"id": "...", "kind": "service", "name": "svc-b", "namespace": "default", "cluster": "karthos-test", "labels": {...}}
        }
      ],
      "inbound": [...]
    }
  }
}
```

Returns 404 if node not found.

### GET /api/v1/graph/edges

List edges from one selected materialized graph with optional filtering and
pagination. By default this is the latest snapshot; `at` selects the newest
retained snapshot at or before a requested RFC3339 instant.
Supplying `from` and `to` instead returns observed edge writes in that
window, including relations no longer present in the current snapshot.
`source_id` filters their stamped source. This form returns observation records
and may repeat one edge ID; it does not accept `at` or a snapshot cursor.

| Param | Type | Description |
|-------|------|-------------|
| `source` | canonical entity ID | Filter by source entity ID |
| `target` | canonical entity ID | Filter by target entity ID |
| `relation` | string | Filter by relation type (`calls`, `runs_on`, `contains`, `exposes`, `routes_to`, `uses`, `mounts`) |
| `at` | RFC3339 timestamp | Select the newest retained snapshot at or before this instant |
| `from`, `to` | RFC3339 timestamps | Inclusive observation window, at most seven days; supply together |
| `source_id` | string | Source stamp filter for observation windows, e.g. `mesh.istio` |
| `page_size` | int | Items per page (default 100) |
| `next_token` | string | Opaque pagination cursor bound to the selected snapshot |

```bash
curl -s "http://localhost:18081/api/v1/graph/edges?relation=calls" | jq .
```

Response:

```json
{
  "data": {"snapshot": {"id": 481, "created_at": "..."}, "edges": [...], "total": 12}
}
```

Window response: `{"data":{"from":"...","to":"...","edges":[...],"total":12}}`.

## Metrics

### GET /api/v1/metrics

Query metric samples with a mode-aware unit. Sorted by timestamp descending.

Semconv metric names are the API contract. Canonical counter samples use base
units (`count`, `bytes`, or `seconds`), while rate units are created only by
query aggregation. Cumulative samples retain their running base total and delta
samples retain their reported interval increment. Every catalog counter declares
its supported/default aggregation and base/rate units. `latest` is the raw-value
aggregation mode: it evaluates the raw selector on the API's fixed 15-second
provider-neutral trend grid, returning evaluation points rather than every
producer sample. Each point retains its cumulative-total or
delta-increment temporality. `increase` is the query-time event/resource change
in its base unit; `window_rate` divides that exact change by the full window
duration; and `active_rate` is available only for cumulative series. Cumulative
`increase` and `window_rate` emit no sample until at least two current-window
samples exist and stored history spans the requested window. Delta series use
`sum_over_time()` for exact increases and return no data for `active_rate`.

For cardinality control, normalized Prometheus application metrics drop raw
`path` and volatile target `pod` / `instance` labels. They retain a
bounded `http_route` only when the application or collector supplied
`http.route` / `http_route`; the normalizer never derives it from a raw path.
Without that route, the canonical application metric is service-level
aggregation rather than per-path data.

- `k8s.pod.phase` -> receiver enum translated at the API boundary: `1=pending`, `2=running`, `3=succeeded`, `4=failed`, `5=unknown`.
- `k8s.pod.status_reason` -> receiver enum translated at the API boundary: `1=evicted`, `2=node_affinity`, `3=node_lost`, `4=shutdown`, `5=unexpected_admission_error`, `6=unknown`. Consumers render `value_label`, never the numeric value.
- `http.server.request.duration` / `servicegraph.request.duration` -> temporality-aware `<metric>_sum` rate divided by the corresponding `<metric>_count` rate

For enum metrics, consumers use the API `value_label` display field. This API
reference documents the numeric mappings; the catalog remains the metric
inventory and semantics contract rather than an enum-mapping surface.

| Param | Type | Description |
|-------|------|-------------|
| `entity_id` | string (repeatable) | Filter by one or more entity IDs |
| `edge_source_id` | string | Filter by edge source entity ID for servicegraph-derived metrics |
| `edge_target_id` | string | Filter by edge target entity ID for servicegraph-derived metrics |
| `metric_name` | string | **Required.** Semconv metric name |
| `from` | string (RFC3339) | Start of time range; omitted uses one hour before the resolved end |
| `to` | string (RFC3339) | End of time range; omitted uses the current time |
| `aggregation` | `latest \| window_rate \| active_rate \| increase` | Counter aggregation; omitted uses the metric's catalog default |
| `window` | Go duration, `1m` through `1h` | Required lookbehind for windowed modes; omitted uses the catalog default |
| `page_size` | int | Items per page (default 100) |
| `next_token` | string | Pagination cursor |

```bash
curl -s "http://localhost:18081/api/v1/metrics?metric_name=servicegraph.request.count&aggregation=window_rate&window=1m&page_size=5" | jq .data.samples[:3]
```

Response:

```json
{
  "data": {
    "query": {"aggregation": "window_rate", "window": "1m", "from": "2026-03-30T09:00:00Z", "to": "2026-03-30T10:00:00Z", "step": "15s", "timestamp_semantics": "evaluation"},
    "samples": [
      {
        "metric_name": "servicegraph.request.count",
        "entity_id": "...",
        "value": 7.5,
        "timestamp": "2026-03-30T10:00:00Z",
        "unit": "per_second",
        "labels": {"edge_client": "service-a", "edge_server": "service-b", "source": "traces.servicegraph", "connection_type": "database", "virtual_node": "server", "client_http_method": "GET", "server_http_method": "GET", "failed": "false"}
      }
    ],
    "total": 100
  },
  "next_token": "NQ=="
}
```

For `k8s.pod.phase` and `k8s.pod.status_reason`, samples carry a `value_label` translated from the integer value. The phase label reports Kubernetes lifecycle state; graph/status health remains a separate derived view and is not a queryable metric.

Health composites use `healthy | running_not_ready | restarting | pending | succeeded | failed | unknown`; Pod phases use `pending | running | succeeded | failed | unknown`; pod status reasons use `evicted | node_affinity | node_lost | shutdown | unexpected_admission_error | unknown`. Canonical mappings live in `pkg/core/metrics/health_status.go`. The `value_label` field is omitted on non-enum metrics.

For edge-oriented metrics, samples include an optional `edge` object plus the original edge labels:

```json
{
  "metric_name": "servicegraph.request.duration",
  "entity_id": "...",
  "value": 0.012,
  "timestamp": "2026-03-30T10:00:00Z",
  "unit": "seconds",
  "edge": {
    "source_id": "svc-a-id",
    "target_id": "svc-b-id"
  },
  "labels": {
    "edge_client": "svc-a",
    "edge_server": "svc-b",
    "edge_client_id": "svc-a-id",
    "edge_server_id": "svc-b-id",
    "connection_type": "messaging_system",
    "virtual_node": "server",
    "client_http_method": "POST",
    "server_http_method": "GET",
    "failed": "false"
  }
}
```

Samples preserve the public normalized `labels` map. Host network samples expose `direction` and `network_interface_name`; volume samples expose `volume_name`, `volume_type`, and `persistentvolumeclaim_name` when PVC-backed. Servicegraph-derived edge metrics include both the human-readable `edge_client` / `edge_server` labels and the stable `edge_client_id` / `edge_server_id` labels so consumers can join runtime traffic, latency, and error signals onto topology edges without a separate edge-metrics endpoint. Bounded source provenance also includes boolean `failed` as the strings `"true"` / `"false"`, `connection_type` (absent, `messaging_system`, or `database`), separate `virtual_node` (`client` or `server`), and distinct client/server HTTP methods when the collector emits them; these labels define separate raw series. True incremental samples carry `temporality=delta`; collector v0.156 servicegraph series are cumulative and therefore do not carry that label. Istio mesh-derived samples use the same edge labels and set `source=mesh.istio`; mesh labels such as `network.protocol.name`, `mesh.security.policy`, `mesh.peer.source.principal`, and `mesh.peer.destination.principal` are preserved when present.

Range-result timestamps are fixed-grid evaluation times, not claims about the producer's original observation instant. The precise `from` and `to` bounds are preserved; `to` is included as an evaluation point even when it is not aligned to the 15-second grid. A value is omitted when its newest raw observation is over five minutes old. Rates, increases, and histogram means are computed from raw observations before their results are placed on the grid. Results are ordered by timestamp descending, then stable public sample fields before pagination. Returns 400 for invalid/inverted time ranges, invalid duration syntax, windows outside `1m..1h`, unknown modes, or controls unsupported by the selected metric. Histogram means accept `window` only; gauges and enum metrics reject both controls.

### GET /api/v1/metrics/series

Metric samples grouped by metric, entity, edge, complete public label set, and resolved unit, with catalog metadata (unit, category, description). Pagination applies at the series level.

| Param | Type | Description |
|-------|------|-------------|
| `entity_id` | string (repeatable) | Filter by one or more entity IDs |
| `edge_source_id` | string | Filter by edge source entity ID for servicegraph-derived metrics |
| `edge_target_id` | string | Filter by edge target entity ID for servicegraph-derived metrics |
| `metric_name` | string | **Required.** Semconv metric name |
| `from` | string (RFC3339) | Start of evaluated range; omitted uses one hour before the resolved end |
| `to` | string (RFC3339) | End of evaluated range; omitted uses the current time |
| `aggregation` | `latest \| window_rate \| active_rate \| increase` | Counter aggregation; omitted uses the metric's catalog default |
| `window` | Go duration, `1m` through `1h` | Lookbehind for windowed modes |
| `page_size` | int | Series per page (default 100) |
| `next_token` | string | Pagination cursor |

```bash
curl -s "http://localhost:18081/api/v1/metrics/series?metric_name=k8s.pod.cpu.usage" | jq .data.series[0]
```

Response:

```json
{
  "data": {
    "series": [
      {
        "metric_name": "k8s.pod.cpu.usage",
        "unit": "cores",
        "category": "resource",
        "description": "CPU usage in cores",
        "samples": [
          {
            "metric_name": "k8s.pod.cpu.usage",
            "entity_id": "...",
            "value": 0.25,
            "timestamp": "2026-03-30T10:00:00Z",
            "unit": "cores",
            "labels": {"source": "kubelet"}
          }
        ]
      }
    ],
    "total": 1
  }
}
```

Different entities, edges, dimension labels, or units produce separate series. The response `query` object exposes the effective range, `15s` step, and `timestamp_semantics: "evaluation"` for both metric endpoints.

### GET /api/v1/metrics/catalog

List semconv-native metric definitions.

| Param | Type | Description |
|-------|------|-------------|
| `category` | string | Filter by category (`resource`, `traffic`, `latency`, `error`, `capacity`, `health`) |

Response entries include a `scopes` array describing whether each semconv metric supports entity queries, edge queries, or both; an `applies_to` array describing the graph entity kinds an entity-scoped metric is valid for; and a `sources` array describing which logical sources can produce the metric and at what priority. Edge-only metrics omit `applies_to`.

The `sources` array drives runtime arbitration: the generic resolver picks the highest-priority (smallest `priority` value) source whose compiled capability is available. The source plan records that result; the generic collector composer includes selected semantic requirements and the OTel adapter renders them, while the ingester drops samples whose `karthos_source` attribute disagrees with the mounted assignment. See [metrics-pipeline.md](metrics-pipeline.md) for the full pipeline.

The array is exposed in priority order (lower number = higher priority); UIs that need a single "primary source" label should render `sources[0].id`. The full array is informational — lower-priority entries are fallbacks the resolver did not pick. The currently-active source per metric is not exposed on the catalog endpoint today; consumers that need it should read the `source` label on actual sample responses from `/api/v1/metrics/series`.

Example:

```bash
curl -s http://localhost:18081/api/v1/metrics/catalog | jq '.data.metrics[] | {name, scopes, applies_to, sources}'
```

```json
{
  "name": "servicegraph.request.count",
  "scopes": ["entity", "edge"],
  "applies_to": ["service"],
  "sources": [
    {"id": "mesh.istio", "priority": 1},
    {"id": "traces.servicegraph", "priority": 2}
  ]
}
```

```bash
curl -s http://localhost:18081/api/v1/metrics/catalog | jq '.data.metrics[].name'
curl -s "http://localhost:18081/api/v1/metrics/catalog?category=latency" | jq .
```

Response:

```json
{
  "data": {
    "metrics": [
      {"name": "k8s.pod.cpu.usage", "unit": "cores", "category": "resource", "description": "..."}
    ],
    "total": 50
  }
}
```

### GET /api/v1/metrics/catalog/stats

Return per-`(canonical metric, source)` sample counts and last-seen timestamps over a fixed 1h lookback. One entry per canonical metric is always returned, zero-filled across the metric's declared sources so the frontend renders coverage badges without a second round-trip. The window is hard-coded (`catalogStatsLookback` in `handler_metrics.go`); take no parameters.

```bash
curl -s http://localhost:18081/api/v1/metrics/catalog/stats | jq '.data.stats[] | select(.metric_name == "servicegraph.request.count")'
```

```json
{
  "metric_name": "servicegraph.request.count",
  "by_source": [
    { "source_id": "mesh.istio",          "priority": 1, "sample_count": 1842, "last_seen_at": "2026-05-16T11:34:56Z" },
    { "source_id": "traces.servicegraph", "priority": 2, "sample_count": 0,    "last_seen_at": null }
  ]
}
```

The endpoint backs the **Catalog** tab in the Metrics Explorer (`/metrics?mode=catalog`), which surfaces coverage badges (live / stale / no-data) so silent advertised-but-not-flowing metrics become visible.

## Entities

### GET /api/v1/entities

List entities with optional filtering and pagination.

| Param | Type | Description |
|-------|------|-------------|
| `kind` | string | Filter by entity kind |
| `namespace` | string | Filter by namespace |
| `page_size` | int | Items per page (default 100) |
| `next_token` | string | Pagination cursor |

```bash
curl -s "http://localhost:18081/api/v1/entities?kind=pod&namespace=default" | jq .
```

Response:

```json
{
  "data": {
    "entities": [
      {"id": "...", "cluster_name": "...", "namespace": "default", "name": "pod-a", "labels": {...}}
    ],
    "total": 42
  }
}
```

### GET /api/v1/entities/{id}

Get a single entity by ID. Returns 404 if not found.

```bash
curl -s http://localhost:18081/api/v1/entities/<entity-id> | jq .data
```

Response:

```json
{
  "data": {
    "id": "...",
    "cluster_name": "karthos-test",
    "namespace": "karthos-demo",
    "name": "service-a-python",
    "labels": {"service.name": "service-a-python"}
  }
}
```

## Implementation Files

When modifying or adding API endpoints, update these files:

| File | Purpose |
|------|---------|
| `pkg/adapters/api/server.go` | Server struct, config, mux wiring, lifecycle |
| `pkg/adapters/api/response.go` | JSON response envelope, pagination, and shared helpers |
| `pkg/adapters/api/handler_status.go` | Status and discovery handlers |
| `pkg/adapters/api/health_resolver.go` | Shared graph and status health derivation |
| `pkg/adapters/api/handler_graph.go` | Graph query handlers with entity enrichment |
| `pkg/adapters/api/handler_metrics.go` | Metrics query, catalog, and series handlers |
| `pkg/adapters/api/handler_entities.go` | Entity query handlers |
| `cmd/backend/main.go` | API server wiring into the backend binary |
| `deploy/helm/karthos/values.yaml` | `backend.service.apiPort`, `backend.statusStaleSeconds`, and `backend.structureRefreshInterval` values |
| `deploy/helm/karthos/templates/backend-deployment.yaml` | API port arg and containerPort |
| `deploy/helm/karthos/templates/backend-service.yaml` | API service port entry |
| `docs/api-reference.md` | **This file** -- update when endpoints change |
| `docs/openapi.yaml` | Machine-readable routes, parameters, and response shapes |

### Source selection and explicit error views

`GET /api/v1/metrics` and `/api/v1/metrics/series` accept optional `source` identifying a registered metric source (for example `mesh.istio` or `traces.servicegraph`). Unknown/non-metric source IDs return400. A registered source without matching data returns no samples. Omission preserves multi-source queries. The effective `query` object echoes an explicitly selected source.

The existing `servicegraph.request.error.count` name is a source-owned query view. For Istio it reads status-partitioned request counters with policy `istio.request-errors` version1; for trace servicegraphs it reports connector-native failures under `servicegraph.connector-failures` version1. These interpretations are distinct. Samples optionally include:

```json
"derivation": {
  "id": "istio.request-errors",
  "version": "1",
  "input_metric": "servicegraph.request.count",
  "interpretation": "HTTP 500–599 for non-gRPC; nonzero gRPC status for RPCs; missing RPC status unknown"
}
```

Existing latest/increase/window-rate/active-rate modes and mode-aware units remain available. Status/source/edge dimensions remain separate. Successful classified partitions may return zero; unknown or absent outcomes do not synthesize zeros. `latest` represents a cumulative classified counter, not the number of failed requests within a selected interval. Public timestamps are query evaluation points, not producer timestamps.
