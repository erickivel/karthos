# API Reference

REST API served by the backend on port **8080** (configurable via `--api-listen-address` / `OKSG_BACKEND_API_LISTEN_ADDRESS`).

For local e2e verification, prefer a dedicated local port-forward such as `18081:8080` so the backend API does not collide with the `gateway-go` traffic port-forward on `18080`.

All endpoints return JSON. Success responses use `{"data": ...}`, errors use `{"error": "...", "code": N}`.

Counts and example IDs in the responses below are illustrative. The live values vary with the currently ingested discovery, metric, and topology data.

## Status

### GET /api/v1/status

System health and component availability.

```bash
curl -s http://localhost:18081/api/v1/status | jq .
```

Response:

```json
{
  "data": {
    "status": "ok",
    "version": {"major": 1, "minor": 0},
    "components": {
      "metric_store": "available",
      "graph_store": "available",
      "metadata_store": "available",
      "entity_registry": "available"
    },
    "uptime_seconds": 3600
  }
}
```

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

Full latest topology snapshot.

```bash
curl -s http://localhost:18081/api/v1/graph | jq .data.summary
curl -s http://localhost:18081/api/v1/graph | jq .data.graph.edges[:5]
```

Response includes `summary.node_count` and `summary.edge_count`. Returns 404 if no snapshot exists.

Response shape:

```json
{
  "data": {
    "version": {"major": 1, "minor": 0},
    "from": "2026-04-02T15:11:12Z",
    "to": "2026-04-02T15:22:12Z",
    "created_at": "2026-04-02T15:22:12Z",
    "graph": {
      "timestamp": "2026-04-02T15:22:12Z",
      "nodes": [...],
      "edges": [...]
    },
    "summary": {
      "node_count": 47,
      "edge_count": 76
    }
  }
}
```

### GET /api/v1/graph/nodes

List graph nodes with optional filtering.

| Param | Type | Description |
|-------|------|-------------|
| `kind` | string | Filter by entity kind (`pod`, `service`, `node`, `workload`, etc.) |
| `namespace` | string | Filter by the node label `namespace` |

```bash
curl -s "http://localhost:18081/api/v1/graph/nodes?kind=service" | jq .
curl -s "http://localhost:18081/api/v1/graph/nodes?namespace=default" | jq .data.total
```

Response:

```json
{"data": {"nodes": [...], "total": 5}}
```

### GET /api/v1/graph/nodes/{id}

Single node by entity ID with its inbound and outbound edges.

```bash
curl -s http://localhost:18081/api/v1/graph/nodes/<entity-id> | jq .
```

Response:

```json
{
  "data": {
    "node": {"id": "...", "kind": "service", "labels": {...}},
    "edges": {
      "outbound": [...],
      "inbound": [...]
    }
  }
}
```

Returns 404 if node not found.

### GET /api/v1/graph/edges

List topology edges with optional filtering.

| Param | Type | Description |
|-------|------|-------------|
| `source` | string | Filter by source entity ID |
| `target` | string | Filter by target entity ID |
| `relation` | string | Filter by relation type (`calls`, `runs_on`, `contains`, `exposes`, `routes_to`, `uses`, `mounts`) |

```bash
curl -s "http://localhost:18081/api/v1/graph/edges?relation=calls" | jq .
curl -s "http://localhost:18081/api/v1/graph/edges?source=<entity-id>" | jq .data.edges[:3]
```

Response:

```json
{"data": {"edges": [...], "total": 12}}
```

## Metrics

### GET /api/v1/metrics

Query metric samples.

| Param | Type | Description |
|-------|------|-------------|
| `entity_id` | string (repeatable) | Filter by one or more entity IDs |
| `metric_name` | string | Filter by canonical metric name |
| `from` | string (RFC3339) | Start of time range |
| `to` | string (RFC3339) | End of time range |

```bash
curl -s "http://localhost:18081/api/v1/metrics?metric_name=cpu_usage_millicores&from=2026-03-30T00:00:00Z" | jq .data.samples[:5]
curl -s "http://localhost:18081/api/v1/metrics?entity_id=abc&entity_id=def" | jq .
```

Response:

```json
{"data": {"samples": [...], "total": 100}}
```

Returns 400 if `from` or `to` is not valid RFC3339.

### GET /api/v1/metrics/catalog

List canonical metric definitions.

| Param | Type | Description |
|-------|------|-------------|
| `category` | string | Filter by category (`resource`, `traffic`, `latency`, `error`, `capacity`) |

```bash
curl -s http://localhost:18081/api/v1/metrics/catalog | jq .data.metrics[].name
curl -s "http://localhost:18081/api/v1/metrics/catalog?category=latency" | jq .
```

Response:

```json
{
  "data": {
    "metrics": [
      {"name": "cpu_usage_millicores", "unit": "millicores", "category": "resource", "description": "..."}
    ],
    "total": 27
  }
}
```

## Entities

### GET /api/v1/entities

List entities from the entity registry.

| Param | Type | Description |
|-------|------|-------------|
| `kind` | string | Filter by entity kind |
| `namespace` | string | Filter by namespace |

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
    "kind": "service",
    "cluster": "oksg-test",
    "namespace": "oksg-demo",
    "name": "service-a-python",
    "labels": {
      "service.name": "service-a-python"
    }
  }
}
```

## Implementation Files

When modifying or adding API endpoints, update these files:

| File | Purpose |
|------|---------|
| `pkg/adapters/api/server.go` | Server struct, config, mux wiring, lifecycle |
| `pkg/adapters/api/response.go` | JSON response envelope and shared helpers |
| `pkg/adapters/api/handler_status.go` | Status and discovery handlers |
| `pkg/adapters/api/handler_graph.go` | Graph query handlers |
| `pkg/adapters/api/handler_metrics.go` | Metrics query and catalog handlers |
| `pkg/adapters/api/handler_entities.go` | Entity query handlers |
| `cmd/backend/main.go` | API server wiring into the backend binary |
| `deploy/helm/oksg-otel/values.yaml` | `backend.service.apiPort` value |
| `deploy/helm/oksg-otel/templates/backend-deployment.yaml` | API port arg and containerPort |
| `deploy/helm/oksg-otel/templates/backend-service.yaml` | API service port entry |
| `docs/api-reference.md` | **This file** -- update when endpoints change |
