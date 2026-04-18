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

System health, component availability, and snapshot freshness.

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
      "nodes": {"healthy": 3, "unhealthy": 0},
      "pods": {"pending": 0, "running": 12, "succeeded": 0, "failed": 0}
    }
  }
}
```

`health_summary` is present when a MetricStore is configured and `node_condition_ready` / `pod_phase` metrics are available. It aggregates the latest samples per entity from the last 5 minutes. `pod_phase` follows the OTel `k8sclusterreceiver` encoding: `1=Pending, 2=Running, 3=Succeeded, 4=Failed, 5=Unknown`.

Status values:

| Value | Meaning |
|-------|---------|
| `nominal` | All stores available AND snapshot fresh |
| `degraded` | Stores available but snapshot stale (> `--status-stale-seconds`), non-critical stores unavailable, or unhealthy nodes detected |
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
    "nodes": [
      {"id": "...", "kind": "pod", "name": "pod-a", "namespace": "default", "cluster": "oksg-test", "status": "unknown", "labels": {...}}
    ],
    "total": 18
  },
  "next_token": "MTA="
}
```

Nodes are enriched from the entity registry with `name`, `namespace`, `cluster`. `status` is `"unknown"` (health derivation planned).

### GET /api/v1/graph/nodes/{id}

Single node with enriched inbound/outbound edges including related entity summaries.

```bash
curl -s http://localhost:18081/api/v1/graph/nodes/<entity-id> | jq .
```

Response:

```json
{
  "data": {
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

Query metric samples with unit from catalog. Sorted by timestamp descending.

| Param | Type | Description |
|-------|------|-------------|
| `entity_id` | string (repeatable) | Filter by one or more entity IDs |
| `edge_source_id` | string | Filter by edge source entity ID for servicegraph-derived metrics |
| `edge_target_id` | string | Filter by edge target entity ID for servicegraph-derived metrics |
| `metric_name` | string | **Required.** Canonical metric name |
| `from` | string (RFC3339) | Start of time range |
| `to` | string (RFC3339) | End of time range |
| `page_size` | int | Items per page (default 100) |
| `next_token` | string | Pagination cursor |

```bash
curl -s "http://localhost:18081/api/v1/metrics?metric_name=cpu_usage_millicores&page_size=5" | jq .data.samples[:3]
```

Response:

```json
{
  "data": {
    "samples": [
      {
        "metric_name": "request_rate_per_second",
        "entity_id": "...",
        "value": 7.5,
        "timestamp": "2026-03-30T10:00:00Z",
        "unit": "per_second",
        "labels": {"edge_client": "service-a", "edge_server": "service-b", "source": "otlp"}
      }
    ],
    "total": 100
  },
  "next_token": "NQ=="
}
```

For edge-oriented metrics, samples include an optional `edge` object plus the original edge labels:

```json
{
  "metric_name": "edge_latency_nanoseconds",
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
    "edge_server_id": "svc-b-id"
  }
}
```

Samples preserve the raw normalized `labels` map. Servicegraph-derived edge metrics include both the human-readable `edge_client` / `edge_server` labels and the stable `edge_client_id` / `edge_server_id` labels so the frontend can join runtime traffic, latency, and error signals onto topology edges without a separate edge-metrics endpoint.

Returns 400 if `from` or `to` is not valid RFC3339.

### GET /api/v1/metrics/series

Metric samples grouped by metric name with catalog metadata (unit, category, description). Pagination applies at the series level.

| Param | Type | Description |
|-------|------|-------------|
| `entity_id` | string (repeatable) | Filter by one or more entity IDs |
| `edge_source_id` | string | Filter by edge source entity ID for servicegraph-derived metrics |
| `edge_target_id` | string | Filter by edge target entity ID for servicegraph-derived metrics |
| `metric_name` | string | **Required.** Canonical metric name |
| `from` | string (RFC3339) | Start of time range |
| `to` | string (RFC3339) | End of time range |
| `page_size` | int | Series per page (default 100) |
| `next_token` | string | Pagination cursor |

```bash
curl -s "http://localhost:18081/api/v1/metrics/series?metric_name=cpu_usage_millicores" | jq .data.series[0]
```

Response:

```json
{
  "data": {
    "series": [
      {
        "metric_name": "cpu_usage_millicores",
        "unit": "millicores",
        "category": "resource",
        "description": "CPU usage in millicores",
        "samples": [
          {
            "metric_name": "cpu_usage_millicores",
            "entity_id": "...",
            "value": 250,
            "timestamp": "2026-03-30T10:00:00Z",
            "unit": "millicores",
            "labels": {"source": "kubeletstats"}
          }
        ]
      }
    ],
    "total": 1
  }
}
```

When edge filters are used, series are grouped by metric name and edge identity, so two different service-to-service links produce two separate series entries even if they share the same canonical metric name.

### GET /api/v1/metrics/catalog

List canonical metric definitions.

| Param | Type | Description |
|-------|------|-------------|
| `category` | string | Filter by category (`resource`, `traffic`, `latency`, `error`, `capacity`, `health`) |

```bash
curl -s http://localhost:18081/api/v1/metrics/catalog | jq '.data.metrics[].name'
curl -s "http://localhost:18081/api/v1/metrics/catalog?category=latency" | jq .
```

Response:

```json
{
  "data": {
    "metrics": [
      {"name": "cpu_usage_millicores", "unit": "millicores", "category": "resource", "description": "..."}
    ],
    "total": 50
  }
}
```

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
