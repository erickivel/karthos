# Development Guide


## Prerequisites

- Go 1.26+
- [Task](https://taskfile.dev/)
- Docker
- kind
- kubectl
- helm

## Tech Stack

Keep the main application minimal:

- Go
- Kubernetes Go client for discovery and cluster-side deployment workflows
- `sigs.k8s.io/yaml` for collector config and Helm values rendering
- Prometheus packages only when a concrete Prometheus adapter, parser, or test application needs them
- OTLP packages only when an OTLP adapter or receiver is added
- `github.com/google/uuid` for event identifiers
- `github.com/invopop/jsonschema` for generated schema documentation
- `github.com/jackc/pgx/v5` for PostgreSQL persistence (pure Go, zero CGO)
- `github.com/stretchr/testify` for tests

Do not add large framework dependencies before there is a concrete need for them. See [GUIDELINE.md](../GUIDELINE.md) for the full dependency philosophy.

## Current Structure

```text
cmd/
  discovery-job/   implemented discovery/plan/deploy CLI
  backend/         OTLP ingest, semantic normalization, and REST API runtime
  controller/      placeholder
  schema-gen/      schema helper

pkg/
  adapters/api/          REST API server (handlers + response envelope + pagination)
  adapters/autoinstrumentation/ reserved boundary for OTel operator integration (doc.go only)
  adapters/istio/        reserved boundary for Istio-specific source adapters (doc.go only)
  adapters/kubernetes/   discovery adapter (Prometheus + OTLP + Istio mesh detection)
  adapters/obi/          reserved boundary for eBPF / OBI ingestion (doc.go only)
  adapters/otel/         source planner; Helm values builder; transform/oksg-source processor
  adapters/otlp/         OTLP gRPC and HTTP server adapter
  adapters/prometheus/   reserved boundary for Prometheus / OpenMetrics adapters (doc.go only)
  adapters/storage/      storage factory and config (memory or external backends)
  adapters/storage/postgresql/     PostgreSQL implementations (pgx/v5)
  adapters/storage/victoriametrics/ VictoriaMetrics MetricStore (HTTP API)
  backend/ingest/        envelope build, signal routing, entity resolution,
                         OTLP normalization, structural / mesh / trace topology
                         builders, composite + multi ingestor, metadata helpers
                         (entity_resolver.go, normalizer.go, topology_builder.go,
                          metric_topology_builder.go, mesh_metric_topology_builder.go,
                          composite_topology_builder.go, structural_topology_builder.go,
                          structural_topology_ingestor.go, normalizing_ingestor.go,
                          buffered_topology_ingestor.go, multi_ingestor_test.go,
                          plus otlp.go, ingest.go, semantic.go, metadata.go)
  backend/persistence/   store interfaces (MetricStore, GraphStore, MetadataStore,
                         RawStore, EntityRegistry) and in-memory implementations
  backend/query/         graph and metrics query contracts
  collector/configgen/   Helm values builder
  collector/deployer/    Helm runner
  core/entities/         canonical graph-facing entities and stable IDs
  core/featurestore/     reserved for future model-ready features
  core/logging/          shared JSON stdout logger
  core/metrics/          canonical catalog, dialect maps, source registry,
                         resolver, OTLP semconv mappings, units
                         (catalog.go, sources.go, dialect.go, resolver.go,
                          otel_mappings.go, canonical.go, sample.go, unit.go)
  core/registry/         canonical metric registry contracts
  core/schema/           raw telemetry envelopes, validation, schema versions
  core/semconv/          OTLP semantic-convention attribute key constants
  core/topology/         graph nodes, edges, relations, and snapshots
  discovery/detectors/   Detector interface (Name, Detect → ObservabilitySource)
  discovery/profile/     ClusterProfile, ObservabilitySource, Mesh types

  modeling/
    scenarios/       reserved scenario definitions
    predictors/      predictor interfaces only
    simulation/      reserved simulation boundary

web/
  src/
    api/             thin native fetch wrapper + per-domain API modules
    hooks/           TanStack Query polling hooks (graph, status, metrics, edge metrics, node metrics)
    stores/          Zustand cross-view state (selectedEntityId, connectionStatus)
    types/           TypeScript types matching backend API contracts
    views/
      ClusterOverview/  KPI row, entity breakdown, relation types, runtime health summary, discovery sources
      TopologyGraph/    React Flow canvas + Ownership / Infrastructure / Traffic projections
        layouts/         dagre layout strategies (ownership, infrastructure, traffic) plus shared
                         dagreLayout.ts and sccCondensation.ts helpers
        nodes/           EntityNode, NamespaceNode, NodeMetric (live metric chip)
        edges/           RelationEdge (cable-bundle SVG edge with traffic-aware width)
        FilterToolbar.tsx, LegendPanel.tsx, NodeInspector.tsx, EdgeMetricButton.tsx
        projection.ts    LAYER_SPECS + project() — per-mode kinds/relations + edge filtering
        useGraphLayout.ts synchronous layout dispatcher keyed by ProjectionMode
        useProjectionState.ts URL-search-param-backed mode/granularity/orphans state
        modes.ts         ProjectionMode and Granularity catalog used by the toolbar
        namespaceColor.ts deterministic namespace color palette
      MetricsExplorer/  category tabs, entity/edge selector, MetricCard charts, raw samples table
    components/
      Shell/          floating TopBar, Sidebar, StatusBar
      shared/         DataChip, SegmentedBar, KpiCard
    styles/           CSS custom properties theme system (dark theme)
  Dockerfile.dev     development container (Vite HMR)
  Dockerfile         production multi-stage (Vite build → nginx)
  nginx.conf         SPA fallback + /api/* proxy

deploy/
  helm/              Helm charts and release packaging
  manifests/         raw manifest placeholder
  examples/          example deployment placeholder

test/
  e2e/
    kind/            kind cluster setup scripts and config
    apps/            example microservices for base and Prometheus scenarios
    scenarios/       end-to-end validation scripts and expected outputs
```

## Common Commands

```bash
# Backend (Go)
task fmt
task test
task build
task vet
task lint
task ci

# Frontend (Docker-only — never run npm/node locally)
task web:dev         # start dev container with Vite HMR (port 5173)
task web:build       # build production Docker image
task web:install     # install npm dependencies inside container
task web:typecheck   # run tsc --noEmit inside container
task web:lint        # run ESLint inside container
task web:e2e:install # install Playwright support inside the Docker e2e runner
task web:e2e         # run topology Playwright tests against the live frontend/backend
task web:e2e:update  # update committed topology screenshot baselines
task ci              # backend checks plus frontend Docker build verification
```

## Discovery And Deployment Workflow

```bash
./bin/discovery-job --mode=discover --kubeconfig=$HOME/.kube/config --cluster-name=oksg-test --output=profile.json
./bin/discovery-job --mode=plan --profile=profile.json --values-output=otel-values.yaml --rendered-output=./bin/e2e/rendered --backend-image-repository=localhost:5001/oksg-backend --backend-image-tag=latest
./bin/discovery-job --mode=deploy --profile=profile.json --release-name=oksg-otel --chart-path=./deploy/helm/oksg-otel --namespace=oksg-system --backend-image-repository=localhost:5001/oksg-backend --backend-image-tag=latest
./bin/backend --grpc-listen-address=0.0.0.0:4317 --http-listen-address=0.0.0.0:4318 --health-listen-address=0.0.0.0:13133 --api-listen-address=0.0.0.0:8080 --status-stale-seconds=300 --enabled-signals=metric,trace --cluster-name=oksg-test --topology-flush-interval=60s --node-ttl=24h
# With external storage:
./bin/backend --storage-backend=external --postgres-dsn="postgres://oksg:pass@localhost:5432/oksg" --victoriametrics-url="http://localhost:8428" --raw-store-enabled=true --enabled-signals=metric,trace --cluster-name=oksg-test --topology-flush-interval=60s --node-ttl=24h
```

Notes:

- use file outputs for `discover` and `plan` when you need clean machine-readable artifacts
- `plan` now emits Helm values, not raw manifests
- `deploy` uses Helm, not `kubectl apply`
- collector runtime flags are driven from Helm values; `agent.featureGates` currently enables `receiver.kubeletstats.enableCPUUsageMetrics` to avoid deprecated kubelet CPU utilization metrics
- `--node-ttl` / `OKSG_NODE_TTL` controls topology node aging (default `24h`). Nodes absent from snapshots for longer than this value are hidden from `GetLatest` so deleted entities stop surfacing indefinitely. Set `0` to disable.

## Helm Ownership

The chart owns Kubernetes resource rendering. When changing deployment behavior:

1. update planner output only when collection behavior changes
2. update values builder when deployment intent changes
3. update chart templates when Kubernetes resource shape changes
4. update `values.schema.json` whenever values shape changes
5. update `deploy/images/backend/Dockerfile` and e2e image build flow when backend runtime packaging changes

Do not reintroduce ad hoc manifest rendering in Go.

## Logging

OKSG-owned components log structured JSON to stdout only.

- configure level through `--log-level` or environment defaults
- inspect runtime behavior with `kubectl logs`
- keep operational logging in docs, not buried in ad hoc comments

## Maintenance Rule

When you replace a flow or interface:

- remove dead functions and retired compatibility code
- delete obsolete templates
- rewrite stale docs and task examples in the same change

When you remove a feature or code path, sweep for phantom references in the same commit:

- grep the repo for the feature name, task name, flag, file path, or API symbol
- delete or rewrite tests that only existed to cover the removed behavior
- delete or rewrite docs, diagrams, scenario scripts, and Taskfile entries that still reference it
- confirm `task ci` stays green so the tree never carries half-removed features

## Frontend Dev Loop

`task web:dev` starts the Vite container with HMR, but the frontend still needs the backend API. In a separate terminal, port-forward the in-cluster backend so the dev server's `/api` proxy can reach it:

```bash
kubectl port-forward -n oksg-system deployment/oksg-backend 18081:8080
```

The dev server reads the API base URL at startup. After you change `.env`, Vite config, or anything the container reads only on boot, stop `task web:dev` and re-run it so the changes take effect. Source file edits continue to hot-reload without a restart.

### Topology layout strategies

`web/src/views/TopologyGraph/layouts/` holds one strategy per projection mode. Each implements the synchronous `LayoutStrategy` interface (`layouts/types.ts`). Keep strategies synchronous unless the layout engine is genuinely async; do not wrap synchronous layouts in `useEffect` + `setState`, because React Query metric maps can change identity and create route churn. Per-mode algorithms:

- **Ownership** — deterministic top-down hierarchy. Namespace groups are laid out side by side, and each independent component inside a namespace computes dynamic ranks from ownership relations. Roots start at the top, and each structural target is placed at least one rank below its source, so direct workload children (pods, ConfigMaps, Secrets, and PVCs) can share the next layer. Ownership intentionally excludes runtime `calls` edges; it keeps Kubernetes structural relations (`routes_to`, `exposes`, `contains`, `uses`, `mounts`) so the tree remains a static ownership/declaration view.
- **Infrastructure** — Dagre left-to-right between K8s nodes, with a lightweight grid packer inside each node group so pods stay compact without pulling in a heavier layout engine. Pods carry a 4 px namespace-color left stripe (the `namespaceColor` helper in `web/src/views/TopologyGraph/namespaceColor.ts`).
- **Traffic** — Dagre left-to-right with greedy cycle breaking and tuned rank/node spacing. Granularity is overridden in `projection.ts` so traffic L2/L3 collapse to overview (pod-level traffic edges multiply by replica count and produce unreadable hairballs).

`useGraphLayout` is a thin synchronous dispatcher that picks the strategy by mode and threads `mode` into each entity node's `data` so renderers (e.g. the namespace stripe in `EntityNode.tsx`) can branch on it. For ownership, pure helpers split namespace components, build relation-derived component models, assign dynamic rank positions, and mark edges for ownership routing. React Flow's live source/target handle coordinates are the source of truth for rendered edge endpoints; do not route hierarchy edges from stale layout-box estimates. For traffic and infrastructure, the shared Dagre adapter computes node placement while React Flow renders relation geometry from normal source/target handles.

### Topology layout QA

Before visual topology QA, make sure the data path is live:

```bash
curl -fsS http://localhost:18081/api/v1/status | jq '.data.system_status'
curl -fsS http://localhost:5173/api/v1/status | jq '.data.system_status'
```

Both should return `nominal`, and the app footer should say `BACKEND_OK`. Do not validate screenshots while the footer shows `BACKEND_OFFLINE`, `DISCONNECTED`, or proxy failures. Use Browser Use screenshots as the primary visual signal for custom SVG edges; DOM edge counts can be misleading because React Flow/custom edge internals are not always exposed in accessibility snapshots. Always verify that navigation into `/topology` does not freeze and that ownership, traffic, and infrastructure each render nodes plus visible edges.

## Debugging

```bash
helm lint ./deploy/helm/oksg-otel -f bin/e2e/otel-values.yaml
helm template oksg-otel ./deploy/helm/oksg-otel --namespace oksg-system -f bin/e2e/otel-values.yaml
kubectl logs -n oksg-system -l app.kubernetes.io/name=oksg-otel,app.kubernetes.io/component=agent --tail=30
kubectl logs -n oksg-system -l app.kubernetes.io/name=oksg-otel,app.kubernetes.io/component=gateway --tail=30
kubectl logs -n oksg-system -l app.kubernetes.io/name=oksg-otel,app.kubernetes.io/component=backend --tail=30
```

## Inspecting the Backend Store

The backend exposes debug endpoints on its health port (13133):

- `/debug/metrics` — all metric samples currently in the in-memory store as JSON; when external VictoriaMetrics storage is enabled, use `/api/v1/metrics` or VictoriaMetrics directly instead
- `/debug/graph` — current topology snapshot including accumulated nodes and edges as JSON

Use the task shortcuts:

```bash
# Dump all stored metric samples (port-forwards, curls, then cleans up)
task test:e2e:debug:metrics

# Verify backend REST API access on a dedicated local port
task test:e2e:debug:api

# Dump the current topology graph snapshot
task test:e2e:debug:graph

# Filter backend logs for write confirmations and skipped-entity warnings
task test:e2e:debug:logs
```

Or run the steps manually:

```bash
kubectl port-forward -n oksg-system deployment/oksg-backend 13133:13133 &
curl -s http://localhost:13133/debug/metrics | jq '.[0:10]'
curl -s http://localhost:13133/debug/graph | jq '.graph.edges[0:5]'
kill %1
```

For external-storage e2e runs, prefer:

```bash
curl -s "http://localhost:18081/api/v1/metrics?metric_name=servicegraph.request.count" | jq '.data.samples[0:10]'
curl -s "http://localhost:18081/api/v1/metrics?metric_name=servicegraph.request.duration&edge_source_id=<source>&edge_target_id=<target>" | jq .
curl -s "http://localhost:18081/api/v1/metrics?metric_name=servicegraph.request.error.ratio&edge_source_id=<source>&edge_target_id=<target>" | jq .
```

Notes:

- `http.server.request.count`, `rpc.server.request.count`, `servicegraph.request.count`, `servicegraph.request.error.count`, gRPC message counts, and TCP connection counts are served as 1-minute rates at query time, even though the stored semconv series are counter-derived.
- `servicegraph.request.error.ratio` is query-derived from the served request and error rates.
- Istio metrics enter through `metrics.IstioDialect`, reuse existing servicegraph canonicals where possible, and preserve mesh metadata labels under `mesh.*`; duration histograms are converted from milliseconds to nanoseconds before storage.
- The Metrics Explorer entity selector is driven by the latest graph snapshot, not the historical `/api/v1/entities` registry.

### Reading the graph output

A graph snapshot from `/debug/graph` looks like:

```json
{
  "graph": {
    "nodes": [
      {"id": "...", "kind": "pod", ...}
    ],
    "edges": [
      {"source": "...", "target": "...", "relation": "runs_on", ...},
      {"source": "...", "target": "...", "relation": "calls", ...}
    ]
  }
}
```

Expected edge relations after telemetry flows:
- `runs_on`: pod → node (from kubeletstats resource attributes)
- `contains`: namespace → pod, pod → container (from kubeletstats resource attributes)
- `calls`: service → service (from OTLP trace `peer.service` attributes, when traces are enabled)

### Reading the output

A successful write line in backend logs looks like:
```json
{"level":"info","msg":"wrote metric samples","source":"otlp/grpc","count":3}
```

A skipped envelope (expected for k8s_cluster workload-object metrics and raw host metrics without node context) looks like:
```json
{"level":"warn","msg":"skipping envelope: unresolvable entity","source":"otlp/grpc","resource":{"k8s.deployment.name":"..."}}
```

Skips for `k8s.namespace`, `k8s.deployment`, `k8s.replicaset`, `k8s.daemonset` resources are expected — these entity kinds are not yet part of the canonical model. Skips with `"resource":null` indicate hostmetrics scrapers whose node context was not injected (check the `resource` processor and `K8S_NODE_NAME` env var in the agent DaemonSet).

## REST API (Layer 6)

The backend exposes a versioned REST API on a dedicated port (default 8080) for querying graph state, metrics, entities, and backend status. All list endpoints support cursor-based pagination (`page_size`, `next_token`). See [docs/api-reference.md](api-reference.md) for the complete reference.

For local e2e work, keep the local port split explicit to avoid colliding with `gateway-go` traffic generation:

- `18080` -> `gateway-go`
- `18081` -> backend REST API
- `13133` -> backend debug/health

```bash
# Query system status (nominal/degraded/disconnected)
curl -s http://localhost:18081/api/v1/status | jq '.data.system_status'

# Graph summary with kind/relation counts and available filters
curl -s http://localhost:18081/api/v1/graph | jq '.data.summary'

# Enriched nodes with entity metadata, paginated
curl -s "http://localhost:18081/api/v1/graph/nodes?kind=pod&page_size=10" | jq .

# Node detail with enriched related entities
curl -s http://localhost:18081/api/v1/graph/nodes/<id> | jq '.data.edges.outbound[0].related_node'

# Metric series grouped by name with catalog metadata
curl -s "http://localhost:18081/api/v1/metrics/series?metric_name=k8s.pod.cpu.usage" | jq '.data.series[0]'

# List canonical metric definitions
curl -s http://localhost:18081/api/v1/metrics/catalog | jq

# List entities with pagination
curl -s "http://localhost:18081/api/v1/entities?kind=pod&page_size=10" | jq .
```

In a cluster, port-forward to the backend API port:

```bash
kubectl port-forward -n oksg-system deployment/oksg-backend 18081:8080 &
curl -s http://localhost:18081/api/v1/status | jq '.data.system_status'
kill %1
```
