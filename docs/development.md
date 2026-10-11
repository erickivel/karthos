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
  schema-gen/      schema helper

pkg/
  adapters/api/          REST API server (handlers + response envelope + pagination)
  adapters/kubernetes/   Kubernetes inventory adapter plus live Pod lifecycle validation
  adapters/otel/         OpenTelemetry adapter: renders OTel configuration and source-stamp processor syntax
  adapters/otlp/         OTLP gRPC ingress server adapter
  adapters/storage/      storage factory and config (memory or external backends)
  adapters/storage/postgresql/     PostgreSQL implementations (pgx/v5)
  adapters/storage/victoriametrics/ VictoriaMetrics MetricStore (HTTP API)
  backend/ingest/        generic envelope build, signal routing, entity resolution,
                         source-policy enforcement, normalization orchestration, and
                         metric / mesh / trace topology builders
  backend/persistence/   store interfaces (MetricStore, GraphStore, MetadataStore,
                         EntityRegistry) and in-memory implementations
  backend/query/         provider-neutral metric-range resolution and graph/metric query values
  collector/spec/        provider-neutral typed collection requirements and compiled CollectionPlan
  collector/             generic composition of selected source requirements
  collector/configgen/   Helm values builder
  collector/deployer/    Helm runner
  core/entities/         canonical graph-facing entities and stable IDs
  core/logging/          shared JSON stdout logger
  core/metrics/          canonical catalog, provider-neutral instrument semantics,
                         resolver contracts, samples, aggregation, and units
  core/schema/           raw telemetry envelopes, validation, schema versions
  core/semconv/          OTLP semantic-convention attribute key constants
  core/topology/         graph nodes, edges, relations, and snapshots
  discovery/profile/     capability-only ClusterProfile and bounded evidence types
  sources/               static module inventory and source-owned detection,
                         raw metric parsing/conversion, semantic collection requirements,
                         source-plan contract, and normalization; structure/
                         owns live Kubernetes topology

web/
  src/
    api/             thin native fetch wrapper + per-domain API modules
    hooks/           TanStack Query polling hooks (graph, status, metrics, node metrics)
    stores/          Zustand cross-view state (selectedEntityId, connectionStatus)
    types/           TypeScript types matching backend API contracts
    views/
      ClusterOverview/  KPI row, entity breakdown, relation types, runtime health summary, discovery sources
      TopologyGraph/    React Flow canvas + Ownership / Infrastructure / Traffic projections
        layouts/         per-mode strategies wired through registry.ts: ownershipLayout (custom
                         tree), infrastructureLayout (dagreLayout.ts wrapper around @dagrejs/dagre),
                         elkLayout.ts (elkjs layered orthogonal for traffic)
        nodes/           EntityNode, NamespaceNode, NodeMetric (live metric chip)
        edges/           RelationEdge (graph relation SVG edge)
        FilterToolbar.tsx, LegendPanel.tsx, NodeInspector.tsx, EdgeMetricButton.tsx
        projection.ts    LAYER_SPECS + project() — per-mode kinds/relations + edge filtering
        useGraphLayout.ts dispatcher that resolves mode via registry; sync strategies
                         resolve in the same render, async strategies (ELK) settle through useEffect
        useProjectionState.ts URL-search-param-backed mode/granularity/orphans state
        modes.ts         ProjectionMode and Granularity catalog used by the toolbar
        namespaceColor.ts deterministic namespace color palette
      MetricsExplorer/  single-entity / single-edge explorer
        filter/        GlobalEntitySearch (⌘K), FilterBreadcrumbBuilder, SearchablePopover, applyFilters
        edge/          EdgeSelector (calls-edge picker)
        applicability/ catalog `scopes` + `applies_to` gating tabs and cards by selected kind
        SelectionSummary, MetricCard, InfoTooltip, RawSamplesTable, CategoryTabs, TimeRangeSelector
    components/
      Shell/          floating TopBar, Sidebar, StatusBar
      shared/         DataChip, KpiCard
    styles/           CSS custom properties theme system (dark theme)
  Dockerfile.dev     development container (Vite HMR)
  Dockerfile         production multi-stage (Vite build → nginx)

deploy/
  helm/              Helm charts and release packaging

test/
  e2e/
    kind/            kind cluster setup scripts and config
    apps/            example microservices for base and Prometheus scenarios
    scenarios/       end-to-end validation scripts and expected outputs
```

## Common Commands

Bare `task` lists the supported commands; it does not run an implicit action.

| Task | Purpose |
|---|---|
| `fmt` / `fmt:check` | Format, or only check formatting, across every Go module. |
| `generate` | Regenerate JSON schemas and Go-generated contracts. |
| `vet` / `lint` / `security` | Analyze every Go module. |
| `test` | Run ordinary main-module unit tests only. |
| `test:branding` | Enforce the tracked Karthos identity contract. |
| `test:race` | Run main-module tests with race detection and coverage. |
| `test:postgres` | Run Docker-backed PostgreSQL integration tests. |
| `test:fixtures` | Check shell fixtures, task contracts, and nested Go modules. |
| `build` | Build backend and discovery-job into `bin/` without rewriting source. |
| `tidy` | Run `go mod tidy` across every Go module. |
| `ci` | Run the comprehensive local backend, fixture, build, and frontend gate. |
| `clean` | Remove generated schemas, binaries, coverage, and E2E output. |
| `deploy` | Validate, build, publish, deploy, and verify the current working tree. |
| `undeploy` | Remove the platform release and its harness-owned PostgreSQL PVC. |
| `web:dev` | Start Dockerized Vite HMR on port 5173. |
| `web:check` | Prepare dependencies, typecheck, lint, and build the production image in Docker. |
| `test:cluster:{profile,doctor,tunnel}` | Inspect or validate the selected cluster profile, or open its optional registry tunnel. |
| `test:e2e:{setup,teardown,reset}` | Manage cluster infrastructure and complete E2E cleanup. |
| `test:e2e:scenario:{fallback,prometheus,istio,detector-failure,source-contract-rejection}` | Run complete E2E scenarios. |
| `test:e2e:traffic` / `test:e2e:traffic:watch` | Generate bounded or continuous fixture traffic. |
| `test:e2e:debug:api` / `test:e2e:debug:logs` | Inspect backend status or follow relevant logs. |

The production frontend image contains only static assets. Helm mounts the
release-scoped Nginx ConfigMap that provides the SPA fallback and routes `/api`
to the matching backend Service. Running that image by itself is unsupported.

## Discovery And Deployment Workflow

```bash
task deploy

# Exceptional low-level debugging only:
./bin/discovery-job --mode=discover --kubeconfig=$HOME/.kube/config --cluster-name=karthos-test --output=profile.json
./bin/discovery-job --mode=plan --capability-profile=profile.json --values-output=otel-values.yaml --rendered-output=./bin/e2e/rendered --backend-image-repository=localhost:5001/karthos-backend --backend-image-tag=latest --frontend-image-repository=localhost:5001/karthos-frontend --frontend-image-tag=latest --storage-backend=external --postgresql-mode=bundled --victoriametrics-mode=bundled
./bin/discovery-job --mode=deploy --capability-profile=profile.json --values-output=otel-values.yaml --release-name=karthos --chart-path=./deploy/helm/karthos --namespace=karthos-system --backend-image-repository=localhost:5001/karthos-backend --backend-image-tag=latest --frontend-image-repository=localhost:5001/karthos-frontend --frontend-image-tag=latest --storage-backend=external --postgresql-mode=bundled --victoriametrics-mode=bundled

# Diagnose the Helm-deployed backend; it requires an in-cluster source contract.
kubectl port-forward -n karthos-system deployment/karthos-backend 18081:8080
curl http://localhost:18081/api/v1/status
```

Notes:

- `task deploy` is the normal product deployment command. It runs focused
  validation, gives a dirty tree a unique timestamped tag, builds and publishes
  backend and frontend from the current working tree, discovers the selected
  cluster, applies Helm, waits for every rollout, and verifies image digests.
- Deployment never installs Istio or demo fixtures.

- `--output` writes only the `discover` capability profile. `plan` and `deploy`
  write a values artifact only through `--values-output`.
- `plan` emits Helm values, not raw manifests. It also writes the resolved-plan
  sidecar beside `--values-output` unless `--resolved-plan-output` is supplied.
- `deploy` is the only supported deployment executor: it compiles, validates,
  writes the dynamic values overlay, and applies the release through Helm. It
  needs no post-deployment ConfigMap or rollout step.
- `--collector-debug` (or `KARTHOS_COLLECTOR_DEBUG=true`) adds the fixed,
  development-only `debug/karthos` exporter to every enabled agent and gateway
  pipeline. It writes detailed batches to Collector stdout only; both planes
  intentionally produce duplicate diagnostic output. The exporter is alpha,
  so treat its log format as transient rather than an API contract.
- Without `--storage-backend`, the chart's `memory` storage default remains in
  effect. `--storage-backend=external` requires both `--postgresql-mode` and
  `--victoriametrics-mode` (`bundled` or `remote`). Remote endpoints come from
  the corresponding flags or `KARTHOS_POSTGRES_DSN` and `KARTHOS_VICTORIAMETRICS_URL`.
- Image flags are typed partial overrides. Omitted repository, tag, or
  pull-policy fields retain the chart's static defaults; this includes the
  optional frontend image override.
- the backend is supported only as a Helm workload because it requires the mounted,
  chart-owned capability profile and resolved source plan.
- collector runtime flags are generated from the collector plan; Collector Contrib is pinned to `0.156.0`
- `--node-ttl` / `KARTHOS_NODE_TTL` is the structural freshness guard (default `24h`). The backend refuses to materialize from a direct Kubernetes structure snapshot older than this value. Set `0` to disable the guard. Materialized API snapshots have an independent fixed 24-hour retention window.
- `backend.structureRefreshInterval` controls the in-cluster structural refresh cadence (default `5m`). It is passed as `--structure-refresh-interval` and must remain strictly below `KARTHOS_NODE_TTL`.
- `backend.statusStaleSeconds` controls when the status endpoint calls a graph snapshot stale (default `360`). It intentionally exceeds the five-minute structural refresh interval to tolerate normal scheduling delay.

## Helm Ownership

The chart owns Kubernetes resource rendering. When changing deployment behavior:

1. update planner output only when collection behavior changes
2. update values builder when deployment intent changes
3. update chart templates when Kubernetes resource shape changes
4. update `values.schema.json` whenever values shape changes
5. update `deploy/images/backend/Dockerfile` and e2e image build flow when backend runtime packaging changes

The chart owns one release-scoped `*-source-contract` ConfigMap. Generated
`backend.sourceContract.capabilityProfile` and `.sourcePlan` values become the
two mounted JSON files, and the backend Deployment hashes that rendered
ConfigMap. Changing either compiled artifact therefore rolls the backend
without an imperative restart.

`backend.grpcPort` is the sole backend OTLP transport setting. It defaults to
`4317`; the chart uses it for the backend listener and Service, then injects
the gateway's reserved `KARTHOS_BACKEND_GRPC_ENDPOINT` environment variable. The
chart likewise injects the agent's `KARTHOS_GATEWAY_GRPC_ENDPOINT` from the
release-scoped gateway Service. These endpoints are not general user overrides.

Do not reintroduce ad hoc manifest rendering in Go.

## Logging

The backend logs structured JSON to stdout. Configure a Helm deployment through
`backend.logLevel` (`info`, `warn`, or `error`); direct commands retain their
`--log-level` and `KARTHOS_LOG_LEVEL` inputs. The OTel adapter fixes agent and
gateway Collector self-telemetry to JSON at `info` on stdout, without a Helm
override. Development-only Collector debug-exporter output is transient
diagnostic output, not structured application logging.

- inspect runtime behavior with `kubectl logs`
- keep operational logging in docs, not buried in ad hoc comments

## Maintenance Rule

When you replace a flow or interface:

- remove dead functions and retired code
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
kubectl port-forward -n karthos-system deployment/karthos-backend 18081:8080
```

The dev server reads the API base URL at startup. After you change `.env`, Vite config, or anything the container reads only on boot, stop `task web:dev` and re-run it so the changes take effect. Source file edits continue to hot-reload without a restart.

`task web:dev` is profile-agnostic: `kubectl port-forward` goes through the
API server, so the same command works against the local kind cluster or a
remote k3s cluster reached over SSH. To switch which cluster the dev loop
targets, change `CLUSTER_PROFILE` in `.env` — see
[remote-cluster.md](remote-cluster.md).

### Topology layout strategies

`web/src/views/TopologyGraph/layouts/` holds one strategy per projection mode, registered through `registry.ts` (`LAYOUT_REGISTRY`). Each strategy implements the `LayoutStrategy` interface (`layouts/types.ts`) and may return either a `LayoutResult` synchronously or a `Promise<LayoutResult>`. Keep strategies synchronous unless the layout engine is genuinely async; do not wrap synchronous layouts in `useEffect` + `setState`, because React Query metric maps can change identity and create route churn. Per-mode strategies:

- **Ownership** — deterministic top-down hierarchy. Namespace groups stack vertically, and each independent component inside a namespace computes dynamic ranks from ownership relations. Roots start at the top, and each structural target is placed at least one rank below its source, so direct workload children (pods, ConfigMaps, Secrets, and PVCs) can share the next layer. Ownership intentionally excludes runtime `calls` edges; it keeps Kubernetes structural relations (`routes_to`, `exposes`, `contains`, `uses`, `mounts`) so the tree remains a static ownership/declaration view.
- **Infrastructure** — Dagre left-to-right between K8s nodes, with a lightweight grid packer inside each node group so pods stay compact without pulling in a heavier layout engine. Pods carry a 4 px namespace-color left stripe (the `namespaceColor` helper in `web/src/views/TopologyGraph/namespaceColor.ts`).
- **Traffic** — ELK layered (`RIGHT`) with orthogonal edge routing (`elkjs`). The strategy is async because ELK runs through the WASM/JS engine. Each edge carries `routePoints` from ELK's edge sections plus `routeStyle: 'orthogonal'` and `rawElk: true`; `RelationEdge` renders those points directly. Its three supported flows are Endpoint (Ingress and Service with `routes_to`/`calls`), Workload (adds workload and `exposes`), and Instance (Pod-level `calls` only).

**Measure-then-layout for ELK.** `EntityNode` is content-driven (CSS pins width to 250 px but height varies per kind / metric content). If ELK is fed the static `NODE_HEIGHT_DEFAULT` for every node, edge endpoints land away from the actual rendered handles. To prevent that, `TopologyGraph.tsx` runs a DOM `ResizeObserver` keyed on each `.react-flow__node[data-id]` and writes the observed sizes into `LayoutInput.nodeSizes`; the ELK strategy uses those when present and falls back to the constants on the very first paint. Cached sizes survive snapshot polls because node IDs are stable across `useGraphSnapshot` refetches — only IDs that actually leave the DOM (different layer / filter) are evicted by the paired `MutationObserver`. Do not switch this to `useNodesInitialized` / `useReactFlow().getNodes()` / `useStore(s => s.nodeLookup)` without verifying that the chosen API actually surfaces `node.measured` for our custom EntityNode mounting; in v12 it did not.

**Metric decoupling.** Layout strategies emit structural positions and edge endpoints only — they never read telemetry. `useGraphLayout` runs a synchronous node-only decoration pass after the resolved `LayoutResult`, so the small catalog-validated topology metric layer cannot re-run or cancel layout. Edge telemetry belongs solely to Metrics Explorer.

`useGraphLayout` resolves the strategy via `LAYOUT_REGISTRY` keyed by mode, threads `mode` into each entity node's `data` so renderers (e.g. the namespace stripe in `EntityNode.tsx`) can branch on it, and surfaces async results via internal `useState`/`useEffect` only when the strategy returns a promise. React Flow's live source/target handle coordinates remain the source of truth for ownership and infrastructure edges; do not route those hierarchy edges from stale layout-box estimates. For ELK-routed traffic edges, the engine's `routePoints` win — they are rendered as-is, and live handle coordinates are intentionally ignored to avoid the diagonal artifact that appears when the first/last segment is rewritten from React Flow handles.

### Topology layout QA

Before visual topology QA, make sure the data path is live:

```bash
curl -fsS http://localhost:18081/api/v1/status | jq '.data.system_status'
curl -fsS http://localhost:5173/api/v1/status | jq '.data.system_status'
```

Both should return `nominal`, and the app footer should say `BACKEND_OK`. Do not validate screenshots while the footer shows `BACKEND_OFFLINE`, `DISCONNECTED`, or proxy failures. Use Browser Use screenshots as the primary visual signal for custom SVG edges; DOM edge counts can be misleading because React Flow/custom edge internals are not always exposed in accessibility snapshots. Always verify that navigation into `/topology` does not freeze and that ownership, traffic, and infrastructure each render nodes plus visible edges.

## Debugging

```bash
helm lint ./deploy/helm/karthos -f bin/e2e/otel-values.yaml
helm template karthos ./deploy/helm/karthos --namespace karthos-system -f bin/e2e/otel-values.yaml
kubectl logs -n karthos-system -l app.kubernetes.io/name=karthos,app.kubernetes.io/component=agent --tail=30
kubectl logs -n karthos-system -l app.kubernetes.io/name=karthos,app.kubernetes.io/component=gateway --tail=30
kubectl logs -n karthos-system -l app.kubernetes.io/name=karthos,app.kubernetes.io/component=backend --tail=30
```

## Inspecting Backend Data

Use the versioned REST API for graph and metric inspection. The health port
(`13133`) exposes health checks only; graph and metric data are not debug
endpoints.

Use the task shortcuts:

```bash
# Verify backend REST API access on a dedicated local port
task test:e2e:debug:api

# Filter backend logs for write confirmations and skipped-entity warnings
task test:e2e:debug:logs
```

Or run the steps manually:

```bash
kubectl port-forward -n karthos-system deployment/karthos-backend 18081:8080 &
curl -s "http://localhost:18081/api/v1/metrics?metric_name=servicegraph.request.count" | jq '.data.samples[0:10]'
curl -s http://localhost:18081/api/v1/graph | jq '.data.edges[0:5]'
kill %1
```

For external-storage e2e runs, prefer:

```bash
curl -s "http://localhost:18081/api/v1/metrics?metric_name=servicegraph.request.count" | jq '.data.samples[0:10]'
curl -s "http://localhost:18081/api/v1/metrics?metric_name=servicegraph.request.duration&edge_source_id=<source>&edge_target_id=<target>" | jq .
```

Notes:

- Counter presentation is catalog-driven. Rate-presented counters default to `aggregation=window_rate&window=1m`, while callers may explicitly request `latest`, `increase`, `window_rate`, or `active_rate` when that metric declares the mode. Cumulative counters such as container restarts retain their catalog-declared `latest` default.
- Istio metrics enter through the Istio source parser, reuse existing servicegraph canonicals where possible, and preserve mesh metadata labels under `mesh.*`; duration histograms are converted from milliseconds to nanoseconds before storage.
- The Metrics Explorer entity search and breadcrumb filter are driven by the latest graph snapshot (`useGraphSnapshot`), not the historical `/api/v1/entities` registry. Edge mode lists `calls` edges from the same snapshot.

Expected graph edge relations after telemetry flows:
- `runs_on`: pod → node (from kubeletstats resource attributes)
- `contains`: namespace → pod, pod → container (from direct Kubernetes structure refresh)
- `calls`: service → service (from OTLP trace `peer.service` attributes or source-classified metric evidence)

### Reading the output

A successful write line in backend logs looks like:
```json
{"level":"info","msg":"wrote metric samples","source":"otlp/grpc","count":3}
```

A skipped envelope (expected for raw host metrics without node context or unsupported future Kubernetes resource attributes) looks like:
```json
{"level":"warn","msg":"skipping envelope: unresolvable entity","source":"otlp/grpc","resource":{"k8s.deployment.name":"..."}}
```

Skips with `"resource":null` indicate hostmetrics scrapers whose node context was not injected (check the `resource` processor and `K8S_NODE_NAME` env var in the agent DaemonSet).

The chart unconditionally renders `hostNetwork: true` with
`dnsPolicy: ClusterFirstWithHostNet`. This lets the hostmetrics network scraper
observe node interfaces; it is a fixed collection-plane accuracy policy, not a
generated-values override. CPU, memory, disk, filesystem, and network scrapers
remain enabled.

## REST API (Layer 6)

The backend exposes a versioned REST API on a dedicated port (default 8080) for querying graph state, metrics, entities, and backend status. All list endpoints support cursor-based pagination (`page_size`, `next_token`). See [docs/api-reference.md](api-reference.md) for the complete reference.

For local e2e work, keep the local port split explicit to avoid colliding with `gateway-go` traffic generation:

- `18080` -> `gateway-go`
- `18081` -> backend REST API
- `13133` -> backend health checks

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
kubectl port-forward -n karthos-system deployment/karthos-backend 18081:8080 &
curl -s http://localhost:18081/api/v1/status | jq '.data.system_status'
kill %1
```
