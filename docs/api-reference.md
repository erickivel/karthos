# API Reference

REST API served by the backend on port **8080** (configurable via `--api-listen-address` / `OKSG_BACKEND_API_LISTEN_ADDRESS`).

For local e2e verification, prefer a dedicated local port-forward such as `18081:8080` so the backend API does not collide with the `gateway-go` traffic port-forward on `18080`.

All endpoints return JSON. Success responses use `{"data": ...}`, errors use `{"error": "...", "code": N}`.

## Pagination

All list endpoints support cursor-based pagination:

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

Applies to: `/graph/nodes`, `/graph/edges`, `/entities`, `/metrics`, `/metrics/series`.

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
      "pods": {"status": "complete", "healthy": 12, "running_not_ready": 0, "restarting": 0, "pending": 0, "succeeded": 0, "failed": 0, "unknown": 2}
    }
  }
}
```

`health_summary` is present whenever a graph snapshot exists. Node health uses the latest `k8s.node.condition.ready` samples from the last 5 minutes. Pod buckets are derived from the `oksg.pod.health.status` derived enum metric (the integer value is translated server-side via `pkg/core/metrics/health_status.go`); entities without a recent sample fall through to `unknown`.

Each branch and the aggregate carry a quality value: `complete`, `partial`, or `unavailable`. A successful metric query remains `complete` even when samples are missing. A failed branch becomes `unavailable`, all applicable current entities in that branch become unknown, and `system_status` becomes `degraded` with a generic branch reason. If no MetricStore is configured, the snapshot still produces an `unavailable` summary populated with current graph entity counts. Detailed store errors are logged but not returned, and these enrichment failures keep HTTP 200.

Snapshot-wide health lookups deliberately omit entity-ID filters, so query size stays constant as the graph grows; the API intersects recent results with current graph IDs in Go. The single-node endpoint retains an exact one-entity filter.

The `unknown` bucket can include pod entities that no longer exist in the cluster but have not yet aged out of the entity registry. Structural ingestion only re-emits pods that the discovery refresh observes, while `oksg.pod.health.status` is derived from `k8s.pod.phase` and disappears as soon as the pod is gone — so during the window between deletion and TTL eviction (`OKSG_NODE_TTL`, default 24h), the topology still lists the pod with `status: "unknown"`.

Pod bucket keys match the public health-status string set used elsewhere in the API: `healthy`, `running_not_ready`, `restarting`, `pending`, `succeeded`, `failed`, `unknown`. The pod-scope composition currently only emits `healthy / pending / succeeded / failed / unknown`; `running_not_ready` and `restarting` are observable today at container scope (`oksg.container.health.status`) and roll up to pod scope only after a future `pod_id` labelling change to container metrics.

Status values:

| Value | Meaning |
|-------|---------|
| `nominal` | All stores available AND snapshot fresh |
| `degraded` | Stores available but snapshot stale (> `--status-stale-seconds`), non-critical stores unavailable, unhealthy nodes detected, or a health-summary branch query failed |
| `disconnected` | GraphStore or EntityRegistry unavailable |

Configuration:

| Flag | Env Var | Default | Description |
|------|---------|---------|-------------|
| `--status-stale-seconds` | `OKSG_BACKEND_STATUS_STALE_SECONDS` | `300` | Snapshot staleness threshold in seconds |

### GET /api/v1/discovery

Discovery profile summary. Returns 404 if no profile has been loaded.

```bash
curl -s http://localhost:18081/api/v1/discovery | jq .
```

Response:

```json
{
  "data": {
    "cluster_id": "...",
    "cluster_name": "oksg-test",
    "discovered_at": "2026-03-30T10:00:00Z",
    "node_count": 3,
    "namespace_count": 5,
    "workload_count": 12,
    "service_count": 8,
    "ingress_count": 2,
    "sources": [...],
    "collection_paths": [...],
    "recommended_receivers": [...]
  }
}
```

## Graph

### GET /api/v1/graph

Full latest topology snapshot with expanded summary.

```bash
curl -s http://localhost:18081/api/v1/graph | jq .data.summary
```

Response shape:

```json
{
  "data": {
    "version": {"major": 1, "minor": 0},
    "from": "2026-04-02T15:11:12Z",
    "to": "2026-04-02T15:22:12Z",
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
        "namespaces": ["default", "oksg-demo", "oksg-system"],
        "relations": ["calls", "contains", "runs_on"]
      }
    }
  }
}
```

Returns 404 if no snapshot exists.

### GET /api/v1/graph/nodes

List enriched graph nodes with optional filtering and pagination.

| Param | Type | Description |
|-------|------|-------------|
| `kind` | string | Filter by entity kind (`pod`, `service`, `node`, etc.) |
| `namespace` | string | Filter by namespace |
| `page_size` | int | Items per page (default 100) |
| `next_token` | string | Pagination cursor |

```bash
curl -s "http://localhost:18081/api/v1/graph/nodes?kind=pod&page_size=10" | jq .
```

Response:

```json
{
  "data": {
    "health_enrichment": {"status": "complete", "pods": "complete", "containers": "complete"},
    "nodes": [
      {"id": "...", "kind": "pod", "name": "pod-a", "namespace": "default", "cluster": "oksg-test", "status": "unknown", "labels": {...}}
    ],
    "total": 18
  },
  "next_token": "MTA="
}
```

Nodes are enriched from the entity registry with `name`, `namespace`, `cluster`. Per-node `status` is one of the public health-status string set: `healthy | running_not_ready | restarting | pending | succeeded | failed | unknown`. For Pod and Container nodes the value is the latest sample of `oksg.pod.health.status` / `oksg.container.health.status` translated to its string label by the API (see `pkg/core/metrics/health_status.go`); for kinds without a derived health metric (Service, Node, etc.) the field is `"unknown"`. `health_enrichment` reports aggregate, pod, and container query quality without adding quality fields to each node. The full graph and single-node responses expose the same metadata. Aggregate runtime health is exposed through `/api/v1/status.health_summary` (see above).

### GET /api/v1/graph/nodes/{id}

Single node with enriched inbound/outbound edges including related entity summaries.

```bash
curl -s http://localhost:18081/api/v1/graph/nodes/<entity-id> | jq .
```

Response:

```json
{
  "data": {
    "health_enrichment": {"status": "complete", "pods": "complete", "containers": "complete"},
    "node": {"id": "...", "kind": "service", "name": "svc-a", "namespace": "default", "cluster": "oksg-test", "status": "unknown", "labels": {...}},
    "edges": {
      "outbound": [
        {
          "id": "...", "source": "...", "target": "...", "relation": "calls",
          "confidence": 1.0, "provenance": ["otlp"], "first_seen": "...", "last_seen": "...",
          "related_node": {"id": "...", "kind": "service", "name": "svc-b", "namespace": "default", "cluster": "oksg-test", "labels": {...}}
        }
      ],
      "inbound": [...]
    }
  }
}
```

Returns 404 if node not found.

### GET /api/v1/graph/edges

List topology edges with optional filtering and pagination.

| Param | Type | Description |
|-------|------|-------------|
| `source` | string | Filter by source entity ID |
| `target` | string | Filter by target entity ID |
| `relation` | string | Filter by relation type (`calls`, `runs_on`, `contains`, `exposes`, `routes_to`, `uses`, `mounts`) |
| `page_size` | int | Items per page (default 100) |
| `next_token` | string | Pagination cursor |

```bash
curl -s "http://localhost:18081/api/v1/graph/edges?relation=calls" | jq .
```

Response:

```json
{
  "data": {"edges": [...], "total": 12},
  "next_token": ""
}
```

## Metrics

### GET /api/v1/metrics

Query metric samples with a mode-aware unit. Sorted by timestamp descending.

Semconv metric names are the API contract. Every catalog counter declares its supported/default aggregation and base/rate units. `window_rate` is the exact reset-aware window increase divided by the full window duration; `active_rate` is the observed cumulative-counter slope; `increase` is the event/resource change; and `latest` is the stored cumulative value. Cumulative `increase` and `window_rate` emit no sample until at least two current-window samples exist and stored history spans the requested window. Delta series use `sum_over_time()` for exact increases and return no data for `active_rate`.

`servicegraph.request.error.ratio` divides baseline-gated error/request increases after grouping by request context and excluding `failed`. It emits no result when any participating error or request branch is immature, preventing partial totals. Histogram means use the same validated window and exact sum/count increases.
- `oksg.container.health.status` -> integer-encoded enum (1=healthy, 2=running_not_ready, 3=restarting) derived from `k8s.container.ready` and `rate(k8s.container.restarts[5m])`. Priority order: restarting > not_ready > healthy. Empty result means no recent sample (treat as unknown).
- `oksg.pod.health.status` -> integer-encoded enum (1=healthy, 4=pending, 5=succeeded, 6=failed, 7=unknown) derived from `k8s.pod.phase`. running_not_ready and restarting at pod scope are a follow-up — see `oksg.container.health.status` for those today.
- `k8s.pod.status_reason` -> receiver enum translated at the API boundary: `1=evicted`, `2=node_affinity`, `3=node_lost`, `4=shutdown`, `5=unexpected_admission_error`, `6=unknown`. Consumers render `value_label`, never the numeric value.
- `http.server.request.duration` / `rpc.server.duration` / `servicegraph.request.duration` / `servicegraph.request.body.size` / `servicegraph.response.body.size` -> temporality-aware `<metric>_sum` rate divided by the corresponding `<metric>_count` rate

The integer→label mapping for any enum metric is documented in the catalog row's `Description` field (see `GET /api/v1/metrics/catalog`); UIs should read it from the catalog rather than hard-coding.

| Param | Type | Description |
|-------|------|-------------|
| `entity_id` | string (repeatable) | Filter by one or more entity IDs |
| `edge_source_id` | string | Filter by edge source entity ID for servicegraph-derived metrics |
| `edge_target_id` | string | Filter by edge target entity ID for servicegraph-derived metrics |
| `metric_name` | string | **Required.** Semconv metric name |
| `from` | string (RFC3339) | Start of time range |
| `to` | string (RFC3339) | End of time range |
| `aggregation` | `latest \| window_rate \| active_rate \| increase` | Counter aggregation; omitted uses the metric's catalog default |
| `window` | Go duration, `1m` through `1h` | Required lookbehind for windowed modes; omitted uses the catalog default |
| `page_size` | int | Items per page (default 100) |
| `next_token` | string | Pagination cursor |

```bash
curl -s "http://localhost:18081/api/v1/metrics?metric_name=k8s.pod.cpu.usage&page_size=5" | jq .data.samples[:3]
```

Response:

```json
{
  "data": {
    "query": {"aggregation": "window_rate", "window": "1m"},
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

For enum metrics (`oksg.pod.health.status`, `oksg.container.health.status`, and `k8s.pod.status_reason`), every sample additionally carries a `value_label` field with the public string label translated from the integer `value` by the backend. Frontends should render `value_label`; the integer encoding is internal to storage and PromQL composition only.

```json
{
  "metric_name": "oksg.container.health.status",
  "entity_id": "...",
  "value": 3,
  "value_label": "restarting",
  "timestamp": "2026-05-08T10:00:00Z",
  "unit": "count",
  "labels": {"source": ""}
}
```

Health composites use `healthy | running_not_ready | restarting | pending | succeeded | failed | unknown`; pod status reasons use `evicted | node_affinity | node_lost | shutdown | unexpected_admission_error | unknown`. Canonical mappings live in `pkg/core/metrics/health_status.go`. The `value_label` field is omitted on non-enum metrics.

For edge-oriented metrics, samples include an optional `edge` object plus the original edge labels:

```json
{
  "metric_name": "servicegraph.request.duration",
  "entity_id": "...",
  "value": 12000000,
  "timestamp": "2026-03-30T10:00:00Z",
  "unit": "nanoseconds",
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

Samples preserve the public normalized `labels` map. Kubelet network samples expose `direction` and `network_interface_name`; volume samples expose `volume_name`, `volume_type`, and `persistentvolumeclaim_name` when PVC-backed. Servicegraph-derived edge metrics include both the human-readable `edge_client` / `edge_server` labels and the stable `edge_client_id` / `edge_server_id` labels so consumers can join runtime traffic, latency, and error signals onto topology edges without a separate edge-metrics endpoint. Bounded source provenance also includes boolean `failed` as the strings `"true"` / `"false"`, `connection_type` (absent, `messaging_system`, or `database`), separate `virtual_node` (`client` or `server`), and distinct client/server HTTP methods when the collector emits them; these labels define separate raw series. True incremental samples carry `temporality=delta`; collector v0.156 servicegraph series are cumulative and therefore do not carry that label. Istio mesh-derived samples use the same edge labels and set `source=mesh.istio`; mesh labels such as `network.protocol.name`, `mesh.security.policy`, `mesh.peer.source.principal`, and `mesh.peer.destination.principal` are preserved when present.

ResourceQuota metrics are heterogeneous. Their catalog unit is `resource`, but each sample uses the unit resolved from its `resource` label: CPU is `millicores`; memory, storage, ephemeral storage, and huge pages are `bytes`; object resources and `count/*` are `count`; unknown extended resources remain `resource`. The internal VictoriaMetrics persistence label `oksg_unit` is never included in public labels. A missing or invalid stored unit falls back to the catalog/query unit.

Returns 400 for invalid time/duration syntax, windows outside `1m..1h`, unknown modes, or controls unsupported by the selected metric. Ratios and histogram means accept `window` only; gauges and enum metrics reject both controls.

### GET /api/v1/metrics/series

Metric samples grouped by metric, entity, edge, complete public label set, and resolved unit, with catalog metadata (unit, category, description). Pagination applies at the series level.

| Param | Type | Description |
|-------|------|-------------|
| `entity_id` | string (repeatable) | Filter by one or more entity IDs |
| `edge_source_id` | string | Filter by edge source entity ID for servicegraph-derived metrics |
| `edge_target_id` | string | Filter by edge target entity ID for servicegraph-derived metrics |
| `metric_name` | string | **Required.** Semconv metric name |
| `from` | string (RFC3339) | Start of time range |
| `to` | string (RFC3339) | End of time range |
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
        "unit": "millicores",
        "category": "resource",
        "description": "CPU usage in millicores",
        "samples": [
          {
            "metric_name": "k8s.pod.cpu.usage",
            "entity_id": "...",
            "value": 250,
            "timestamp": "2026-03-30T10:00:00Z",
            "unit": "millicores",
            "labels": {"source": "kubelet"}
          }
        ]
      }
    ],
    "total": 1
  }
}
```

Different entities, edges, dimension labels, or units produce separate series. This is especially important for ResourceQuota: `resource=requests.cpu` (`millicores`), `resource=requests.memory` (`bytes`), and `resource=pods` (`count`) never share a series or numeric axis.

The removed names `k8s.container.storage.request` and `k8s.container.storage.limit` are not catalog metrics and return HTTP 400. Use `k8s.container.ephemeral_storage.{request,limit}` for ephemeral storage and `k8s.volume.*` for mounted volume capacity/usage.

### GET /api/v1/metrics/catalog

List semconv-native metric definitions.

| Param | Type | Description |
|-------|------|-------------|
| `category` | string | Filter by category (`resource`, `traffic`, `latency`, `error`, `capacity`, `health`) |

Response entries include a `scopes` array describing whether each semconv metric supports entity queries, edge queries, or both; an `applies_to` array describing the graph entity kinds an entity-scoped metric is valid for; and a `sources` array describing which logical sources can produce the metric and at what priority. Edge-only metrics omit `applies_to`.

The `sources` array drives runtime arbitration: at backend startup the resolver (`pkg/core/metrics/resolver.go`) picks the highest-priority (smallest `priority` value) source whose detector returns true for the cluster's discovery profile. The planner only deploys receivers for selected sources; the ingester drops samples whose `oksg_source` attribute disagrees with the assigned source. See [metrics-pipeline.md](metrics-pipeline.md) for the full pipeline.

The array is exposed in priority order (lower number = higher priority); UIs that need a single "primary source" label should render `sources[0].id`. The full array is informational — lower-priority entries are fallbacks the resolver did not pick. The currently-active source per metric is not exposed on the catalog endpoint today; consumers that need it should read the `source` label on actual sample responses from `/api/v1/metrics/series`.

`selection_policy` defaults to `"priority"` (omitted from the response when default). The values `"disjoint"` and `"all"` are reserved for future per-scope routing and multi-source aggregation; the resolver returns an error if it encounters them.

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
      {"name": "k8s.pod.cpu.usage", "unit": "millicores", "category": "resource", "description": "..."}
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

The endpoint backs the **Catalog** tab in the Metrics Explorer (`/metrics?mode=catalog`), which surfaces coverage badges (live / stale / no-data / derived) so silent advertised-but-not-flowing metrics become visible.

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
      {"id": "...", "kind": "pod", "cluster": "...", "namespace": "default", "name": "pod-a", "labels": {...}}
    ],
    "total": 42
  },
  "next_token": ""
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
    "kind": "service",
    "cluster": "oksg-test",
    "namespace": "oksg-demo",
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
| `pkg/adapters/api/handler_graph.go` | Graph query handlers with entity enrichment |
| `pkg/adapters/api/handler_metrics.go` | Metrics query, catalog, and series handlers |
| `pkg/adapters/api/handler_entities.go` | Entity query handlers |
| `cmd/backend/main.go` | API server wiring into the backend binary |
| `deploy/helm/oksg-otel/values.yaml` | `backend.service.apiPort` and `backend.statusStaleSeconds` values |
| `deploy/helm/oksg-otel/templates/backend-deployment.yaml` | API port arg and containerPort |
| `deploy/helm/oksg-otel/templates/backend-service.yaml` | API service port entry |
| `docs/api-reference.md` | **This file** -- update when endpoints change |
