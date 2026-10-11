# Agent Guide

This repository builds a normalized observability data layer for Kubernetes.
It ingests OTLP and Kubernetes telemetry, maps heterogeneous signals into
stable canonical entities, metrics, and relationships, preserves raw
provenance and source dimensions, controls cardinality, and exposes generic
APIs for tools built on top of that normalized data.

The graph visualizer is one consumer of this data layer. Future analytics,
automation, simulation, and prediction systems are also consumers. The long-term
goal is a live observability knowledge graph that can explain the current
system and later simulate or predict the performance impact of changes.

See [README.md](README.md) for a project summary and [GUIDELINE.md](GUIDELINE.md) for design principles that govern all contributions.

## Product North Star

Agents working in this repository should optimize for the data foundation, not
only the current UI screen or the shortest local fix.

- Treat canonical storage and APIs as the platform contract. UI, analytics,
  automation, and future prediction tools must consume stable interfaces rather
  than backend internals or source-specific shortcuts.
- Preserve truth first: keep source provenance, dimensions, labels, timestamps,
  entity identity, and transformation context before creating summaries.
- Aggregate intentionally. Node-level, workload-level, or UI-friendly values
  should be query/API transformations over preserved data, not accidental
  flattening during ingest or storage.
- Keep prediction and simulation as future consumers of the canonical
  graph-and-metrics foundation. Do not mix prediction logic into discovery,
  collection, normalization, or storage layers prematurely.
- Prefer generic query capabilities with filters, grouping, aggregation knobs,
  time ranges, and source selectors so different tools can request the data
  shape they need without new bespoke endpoints for each screen.
- Control cardinality deliberately. Preserve stable semantic dimensions needed
  for correctness, but avoid volatile labels, raw paths, generated IDs, or
  unbounded attributes unless a storage/query contract explicitly justifies
  them.
- Keep source-specific and vendor-specific behavior behind small, explicit
  interfaces. Prefer lightweight dialects, adapters, and registries whose
  implementations can be selected or replaced without changing shared domain
  logic. Each implementation should own its source-specific mapping,
  dimensions, conversions, defaults, and tests. Do not introduce a dynamic
  plugin framework unless runtime loading or third-party extensibility is an
  actual requirement; modular boundaries and interchangeable implementations
  are the goal.

## Quick Reference Commands

- `task ci` -- run before and after every meaningful change
- `task test` -- unit tests only
- `task fmt` -- format code
- `task build` -- compile all binaries
- `task generate` -- regenerate JSON Schema output after schema changes
- `task test:cluster:profile` -- print the resolved cluster profile variables
- `task test:cluster:tunnel` -- open the SSH registry tunnel (k3s+ssh profiles only)
- `task test:cluster:doctor` -- engine-aware pre-flight checks for the active profile

## Documentation Map

All implementation details live in `docs/`. Use and update these files when implementation changes.

| Document | What it covers | Update when... |
|----------|---------------|----------------|
| [README.md](README.md) | Project summary, phase status, getting started | Project phase, runtime surface, or entrypoints change |
| [GUIDELINE.md](GUIDELINE.md) | Design principles, configuration rules, dependency philosophy | Principles themselves change (rare) |
| [docs/architecture.md](docs/architecture.md) | System layers, component boundaries, deployment model | Adding or modifying adapters or layers |
| [docs/discovery-flow.md](docs/discovery-flow.md) | Discovery pipeline, sequence diagrams, example outputs | Changing discovery or deployment pipeline |
| [docs/runtime-flow.md](docs/runtime-flow.md) | Post-deploy telemetry flow, component responsibilities | Changing the deployed runtime telemetry path |
| [docs/testing.md](docs/testing.md) | Unit tests, e2e model, scenarios, traffic generation | Changing test infrastructure, task names, or Helm install flow |
| [docs/api-reference.md](docs/api-reference.md) | REST API routes, query params, request/response examples | Adding, removing, or changing API endpoints |
| [docs/development.md](docs/development.md) | Project structure, tech stack, common commands, Helm ownership | Adding packages, changing structure, or modifying workflows |
| [docs/metrics-pipeline.md](docs/metrics-pipeline.md) | Catalog, dialect, source registry, resolver, source arbitration end-to-end | Changing canonical metrics, dialects, sources, or arbitration rules |
| [docs/frontend-design-spec.md](docs/frontend-design-spec.md) | Frontend tech stack, views, design system, projection modes, layout strategies | Changing frontend views, projection logic, layout strategies, or theme tokens |
| [docs/remote-cluster.md](docs/remote-cluster.md) | `CLUSTER_PROFILE` model, supported cells (kind+local, k3s+ssh), setup / teardown / tunnel / doctor flow | Adding or changing a cluster cell, profile variables, or engine dispatch |

## After-Change Checklist

1. Run `task ci`.
2. Ensure unit tests cover any new behavior.
3. If you changed discovery or collection behavior, run the relevant e2e scenarios (see [docs/testing.md](docs/testing.md) for commands and expectations).
4. Update the documentation file(s) from the table above that correspond to what you changed.
5. Remove or rewrite any docs, commands, or diagrams made stale by your change -- in the same commit.

## CI/CD

GitHub Actions runs on PRs and pushes to `main`/`development`:

- **lint**, **format-check**, **vet**, **test**, **build** on all branches
- **security** (govulncheck + gosec) on `main` only

Run `task ci` locally before pushing.

## Upgrading the Go Version

When upgrading Go (e.g., to fix `govulncheck` findings), update **all** of these files:

| File | What to change |
|------|----------------|
| `go.mod` | `go X.Y.Z` directive |
| `.tool-versions` | `golang X.Y.Z` (asdf) |
| `.github/workflows/ci.yml` | `GO_VERSION: "X.Y.Z"` env var |
| `.env.example` | `GO_VERSION` comment |
| `deploy/images/backend/Dockerfile` | `FROM golang:X.Y.Z-alpine` |
| `test/e2e/apps/base/gateway-go/Dockerfile` | `FROM golang:X.Y.Z-alpine` |
| `test/e2e/apps/instrumented/gateway-go/Dockerfile` | `FROM golang:X.Y.Z-alpine` |
| `test/e2e/apps/instrumented/gateway-go/go.mod` | `go X.Y.Z` directive |
| `test/e2e/apps/mesh-protocol-go/Dockerfile` | `FROM golang:X.Y.Z-alpine` |
| `test/e2e/apps/mesh-protocol-go/go.mod` | `go X.Y.Z` directive |

After updating, run `go mod tidy`, reinstall tools (`go install ...`), `asdf reshim golang`, then `task ci` and `task security`.

## Frontend Development Rules

The frontend lives in `web/`. **All frontend operations run inside Docker containers — never install or run npm/node locally.**

### Commands

| Command | What it does |
|---------|-------------|
| `task web:dev` | Start dev container with Vite HMR (port 5173) |
| `task web:check` | Build shared dependencies, typecheck, lint, and build the production image |

### Key rules

- **Never run `npm install`, `npm run`, `npx`, or any node/npm command on the host machine.** Always use Docker (`docker compose run --rm karthos-frontend <command>` or the Taskfile shortcuts).
- Source files are volume-mounted into the dev container for HMR. Edit files on the host, the container picks up changes.
- API requests are proxied: Vite dev server proxies `/api/*` to the backend container, nginx does the same in production.
- The `web/` directory has its own `Taskfile.yml` included from the root.
- Topology layout strategies are synchronous unless a layout engine is genuinely async. Do not convert them to `useEffect` + `setState` just to fit a dispatcher. The Traffic strategy is async because `elkjs` is genuinely async — `useGraphLayout` handles that case via internal state without affecting sync strategies.
- Topology layouts are wired through `LAYOUT_REGISTRY` (`web/src/views/TopologyGraph/layouts/registry.ts`). When adding or replacing an algorithm for a projection mode, register it there with a stable `LayoutAlgorithm` name; do not bypass the registry from `useGraphLayout` or call sites.
- Layout strategies must NOT read `nodeMetrics` / `edgeMetrics`. `useGraphLayout` runs a separate `decorateWithMetrics` pass after layout settles. Re-coupling metrics into a strategy will cause every metric poll to re-run layout (and cancel ELK mid-resolve).
- The DOM `ResizeObserver` in `TopologyGraph.tsx` feeds real measured sizes into `LayoutInput.nodeSizes` so ELK's edge endpoints land on the actual rendered handle box. Do not remove it. Cached sizes intentionally survive snapshot polls (node IDs are stable across `useGraphSnapshot` refetches). If you switch to a React Flow API (`useNodesInitialized`, `useStore(s => s.nodeLookup)`, etc.), verify that `node.measured` is actually populated for the custom `EntityNode` first — in v12 it was not, which is why we observe the DOM directly.
- For topology edges, React Flow's live handle coordinates are the endpoint source of truth. Do not route rendered edges from stale layout-box estimates. **Exception:** edges with `data.rawElk === true` carry an explicit `routePoints` array produced by ELK; render those points as-is and skip live handle anchoring, otherwise the first/last segment tilts off the orthogonal route.
- **Do NOT set `width`/`height` on the React Flow `Node` objects emitted from `trafficElkLayout`** (`web/src/views/TopologyGraph/layouts/elkLayout.ts`). Doing so freezes handle positions to the ELK-time estimate; when the rendered DOM is wider than the estimate, edges visually disconnect from the cards because React Flow stops re-measuring. The Traffic minimap will show empty cells for those nodes — that's the documented trade-off, the layout/edge correctness is the priority. Synchronous layouts (dagre / ownership / infrastructure-nested) DO set explicit `width`/`height` so the minimap can render their nodes; only the ELK strategy must abstain.
- Before judging topology screenshots, verify backend direct and frontend proxy status are both nominal and the footer says `BACKEND_OK`; offline/disconnected screenshots are not valid layout evidence.
- Browser Use DOM/accessibility counts can miss custom SVG edge internals. Treat screenshots and app-state/layout evidence as primary for topology edge verification.
- **Topology projection layer relations** (`web/src/views/TopologyGraph/projection.ts` → `LAYER_SPECS` and `project()`):
  - Ownership relations must include all five of `contains`, `mounts`, `uses`, `exposes`, `routes_to`. Removing any one of them filters Ingress / call-only services out of `computeParticipants` and they vanish from the view (regression: "ownership is missing Ingress/ConfigMap").
  - Do **NOT** add `calls` to the ownership layer relations. `calls` is a runtime traffic relation and belongs to the Traffic layer only; putting it in ownership pollutes the structural view with traffic edges. This rule has flipped before — keep `calls` out of ownership.
  - Traffic relations stay `calls`, `routes_to`, `exposes`. Infrastructure stays `runs_on`, `contains`. PVCs / Secrets / ConfigMaps do not belong in the Infrastructure view — that view is purely "where compute runs" (Node ⊃ Pod ⊃ Container). Don't blend the relation sets.
  - **Infrastructure granularity** (`web/src/views/TopologyGraph/modes.ts` → `INFRASTRUCTURE_GRANULARITY_ENTRIES`): the picker reads "Nodes / Pods on nodes / Containers in pods", but the URL values stay `overview` / `runtime` / `detailed` for shareable-link stability — same convention as Traffic. Don't rename the URL values.
  - **Infrastructure detailed nesting** (`web/src/views/TopologyGraph/layouts/infrastructureLayout.ts`): when containers are visible the layout switches to a hand-packed two-tier nested rectangle layout (containers inside pod rectangles inside node rectangles) and emits `edges: []` — containment is encoded by visual nesting, so drawing `runs_on`/`contains` arrows on top would only add noise. Don't reach for dagre's compound graph here; it only handles one tier. The single-tier (L1/L2) infra cases still go through dagre.
  - The edge filter in `project()` MUST check `spec.relations.has(e.relation)` in addition to endpoint visibility. Without that, removing a relation from a layer's set only changes which entities count as participants — edges of that relation still render between two already-visible nodes. Real symptom: `calls` edges leaked into Ownership even after `calls` was removed from `ownership.relations`, because both endpoint services were already visible via `exposes` / `contains`. Keep both filters in lockstep.
  - **Traffic mode is split into three flows by granularity** via `TRAFFIC_FLOW_SPECS` + `resolveLayerSpec`: `overview` → Endpoint Flow (ingress/service; `routes_to`+`calls`), `runtime` → Workload Flow (+workload; +`exposes`), `detailed` → Instance Flow (pod-only; `calls`). The old `mode === 'traffic'` overview override in `visibleKindsFor` was removed deliberately — re-introducing it would collapse Workload and Instance Flows back to Endpoint Flow. `exposes` only belongs in Workload Flow; do NOT add it to Endpoint or Instance Flow (it is structural, not observed traffic). The picker labels swap via `granularityEntriesFor(mode)`; the underlying URL value (`overview`/`runtime`/`detailed`) is preserved for shareable-link stability.

## Kubernetes Port-Forwarding

When you need to access cluster services locally, use these commands:

| Service | Command |
|---------|---------|
| Karthos Backend API | `kubectl port-forward -n karthos-system deployment/karthos-backend 18081:8080` |

The backend API is then available at `http://localhost:18081/api/v1/...`.

## Documentation Maintenance Rules

- Treat `docs/` as the primary implementation documentation surface.
- Keep documentation aligned with actual implementation, not aspirational or outdated flows.
- Keep diagrams (ASCII and Mermaid) synchronized with implementation.
- Keep `docs/discovery-flow.md` explicit about what auto-detection does today versus what the planner accepts from externally supplied profiles.
- Keep logging documentation in `docs/`: structured JSON logs to stdout only, `kubectl logs` as the debugging path.
- Update example outputs in `docs/discovery-flow.md` when ClusterProfile structure changes.
- When removing a feature or code path, sweep the repo for phantom references in the same commit: tests, docs, diagrams, Taskfile entries, and scenario scripts. See `docs/development.md` → *Maintenance Rule* for the checklist.
- For the frontend dev loop (`task web:dev` with a port-forwarded backend, and when to restart the dev container), see `docs/development.md` → *Frontend Dev Loop*.

## Discovery Refresh & Node TTL Invariant

The Postgres graph store ages out nodes whose most recent containing snapshot
is older than `KARTHOS_NODE_TTL` (default 24h, see `loadLatestObservedNodes` in
`pkg/adapters/storage/postgresql/graph_store.go`). Workload, Ingress,
ConfigMap, Secret, and PVC nodes are produced **only** by the direct
Kubernetes structure module (`pkg/sources/structure`) — runtime OTLP signals
never re-emit them. So:

- The structure module must refresh on a timer strictly shorter than
  `NodeTTL`, otherwise these entity kinds disappear from `/api/v1/graph`
  after the TTL window. This is the regression that produced "ownership is
  missing Ingress / ConfigMap / Workload" reports.
- `cmd/backend/main.go:runStructureRefreshLoop` does this on
  `KARTHOS_STRUCTURE_REFRESH_INTERVAL` (default 10m). DO NOT remove the loop or
  raise the interval to ≥ NodeTTL.
- If you change `defaultNodeTTL` or `defaultStructureRefreshInterval`,
  preserve the strict inequality `discovery_refresh_interval < node_ttl`.
- Pods/containers/services/nodes/namespaces survive without this loop because
  OTLP keeps them fresh; structural-only kinds do not.

## Telemetry Mapping Rules

- Before integrating or changing any collector, connector, receiver, exporter,
  telemetry service, metric, or trace-derived metric, read its authoritative
  upstream documentation and, when semantics depend on implementation details,
  its source code. Check the documentation for the exact deployed version, not
  only the latest release, and record any relevant version differences.
- Record the verified signal contract: how the signal is generated; whether it
  is a gauge, sum, or histogram; whether sums are monotonic; whether temporality
  is cumulative or delta; its units and dimensions; and all behaviorally
  relevant defaults such as collection, scrape, flush, aggregation, expiration,
  cache, pairing, and TTL windows. Confirm which settings are defaults and which
  are explicitly generated by Karthos.
- Treat external documentation review as implementation evidence, not optional
  background reading. Link the authoritative reference in the relevant product
  docs or audit notes, and validate the documented behavior against the live
  generated configuration and emitted telemetry whenever feasible.
- Only after that verification should you change the semconv metric registry, dialect translations, unit conversions, query-time derivations, storage expectations, or UI assumptions.
- Do not infer telemetry semantics from a metric name alone. Connectors such as `servicegraph` can emit counters, histograms, and unit defaults that differ from what the semconv name suggests.
- Isolate collector and service integration behavior in the corresponding
  dialect or adapter instead of adding source-specific branches to shared
  normalization, storage, or API code. Shared code should depend on a narrow
  contract and remain source-agnostic. Register implementations by a stable
  source ID, keep one focused implementation per source when practical, and
  test each implementation independently plus through the shared contract.
- When introducing an integration interface, make the ownership boundary clear:
  document what the shared layer guarantees, what the source implementation
  may customize, how it is selected, and what truthful fallback behavior occurs
  when no implementation is registered. Prefer the smallest interface that
  supports known sources and keep source-specific configuration out of it.
- Storage and API contracts must preserve the dimensions required to make a
  metric truthful. For dimensional telemetry such as CPU state, memory state,
  disk device/direction, filesystem mount/state, network interface/direction,
  or resource quota resource labels, do not collapse labels merely because the
  current UI wants one card value. Build explicit aggregation/query behavior on
  top of preserved series.
- When a consumer needs summarized data, prefer generic API/query controls
  such as `filter`, `group_by`, `aggregate`, `source`, `entity_id`, and time
  range semantics over UI-specific endpoints or storage-time rollups. If a
  named view is added for convenience, it should be documented as a derived
  view over canonical data.
- When an audit or bug investigation finds a metric that is missing, wrong, or
  passed through a layer in the wrong shape, document concrete examples from
  each checked layer: source truth, collector/VM series, stored labels/entity
  IDs, API response values/units/labels, and frontend rendering. These examples
  are part of the fix context so later agents can reproduce the failure and
  decide whether to change collection, normalization, storage, API querying, or
  presentation.
- Every metric pipeline change should consider at least three consumers:
  current frontend surfaces, audit/automation tools, and future feature
  extraction for prediction or simulation. This does not mean implementing
  prediction now; it means keeping the canonical data contract reusable.
- Before adding or modifying any catalog metric, follow the end-to-end checklist in [docs/metrics-pipeline.md § *Adding a New Metric: End-to-End Checklist*](docs/metrics-pipeline.md#7-adding-a-new-metric-end-to-end-checklist). Verify on a live kind cluster — unit tests do not catch receiver-config or OTTL-tag mistakes; metrics can be advertised in the catalog yet silently produce zero datapoints.
- Status/lifecycle signals stay integer-encoded through the pipeline. `MetricSample.Value` is `float64`; do not introduce string-valued samples for enum metrics like `k8s.pod.phase` or `karthos.{pod,container}.health.status`. Translate integer enums to plain-text strings ONLY at the API boundary — the canonical translator is `pkg/core/metrics/health_status.go`, applied by the metric store (sets `MetricSample.ValueLabel`) and the graph/status handlers (populate `enrichedNode.Status` and `health_summary.pods.*` keys). Frontends never see enum integers; the only consumers of the integer form are storage and PromQL composition.
