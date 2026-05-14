# Frontend Design Specification — OKSG Visualizer

The OKSG frontend is a read-only observability dashboard that renders the
topology graph and metrics produced by the OKSG backend. No cluster
mutations, no control-plane operations.

---

## 2. Tech Stack

| Layer | Choice | Rationale |
|-------|--------|-----------|
| Build | Vite | Fast HMR, simple config, static output |
| Framework | React 18+ | Component model, ecosystem, React Flow compatibility |
| Routing | React Router v6 | Layout routes for the persistent shell |
| Graph | React Flow (@xyflow/react v12) | Custom node components, built-in pan/zoom/minimap |
| Charts | Recharts | React-native, composable, dark-theme friendly |
| State | Zustand | Lightweight cross-view shared state |
| HTTP | Native `fetch` | Zero dependencies, built-in to all browsers, no axios |
| Data Fetching | TanStack Query | Polling intervals, caching, stale detection, retry |
| Styling | CSS Modules + CSS Custom Properties | Theme tokens as `var(--*)`, no hardcoded hex in components |
| Font | Space Grotesk (Google Fonts) | Per design system spec |
| Language | TypeScript | Type safety against API contracts |

**No UI framework** (no MUI, Chakra, etc.). The "Graphic Retro-Futurism" design system is custom — flat surfaces, tonal layering, zero shadows/gradients/borders.

---

## 3. Design System Reference

The visual language follows the "Graphic Retro-Futurism / The Analog Superintendent" design system. Key rules:

### Theming Architecture

**All colors are consumed exclusively via CSS custom properties (`var(--*)`).** No component or CSS module may reference a hex value directly. This enables light mode support later by swapping the theme file.

Theme files live in `src/styles/themes/`:
- `dark.css` — default theme, applied via `[data-theme="dark"]` on `<html>`
- `light.css` — future, same variable names with light-mode values

Components reference only semantic tokens like `var(--color-signal)`, `var(--surface-base)`, `var(--text-muted)`, etc.

### Colors (Dark Theme)
- `--color-signal` (`#08dc7d`) — nominal state, primary actions
- `--color-alert` (`#c62d2f`) — critical failures, error state
- `--color-info` (`#3e9adf`) — informational, infrastructure entities
- `--color-text-primary` (`#e0e0e0`) — primary text
- `--color-text-muted` (`#a3a7af`) — secondary metadata, labels

### Surface Hierarchy (tonal layering, no borders)
1. `--surface-base`: `#121416`
2. `--surface-nav`: `#1a1c1e`
3. `--surface-module`: `#1e2022`
4. `--surface-active`: `#333537`

### Typography
- **Font:** Space Grotesk exclusively
- **Headlines:** 1.5rem, tight letter-spacing (-0.02em)
- **Labels:** 0.6875rem, uppercase, wide letter-spacing (0.08em)
- **Body/Data:** 0.75rem

### Rules
- No 1px borders — boundaries via background color shifts only
- No shadows — hierarchy via tonal layering
- No gradients — every element is a flat color token
- **Corner radius: 4-6px** — squared cards with slightly rounded corners
- Icons use `lucide-react`, selecting glyphs with a consistent 1.5-2px stroke width that pairs cleanly with Space Grotesk
- **No hardcoded hex values in components** — all colors via `var(--*)`

---

## 4. Architecture

### Pattern: Route-Based Views with Shared Data Layer

Three routes sharing a persistent shell (sidebar + top bar + status bar). Zustand for cross-view state. TanStack Query for data fetching with polling.

### Application Shell (persistent across all routes)

| Component | Position | Description |
|-----------|----------|-------------|
| **Top Bar** | Floating, top | OKSG branding, polling interval indicator, connection status dot. Floats over the canvas with `var(--surface-nav)` background, not docked to the edge. |
| **Sidebar** | Floating, left | Icon navigation for 3 routes. Floats over the canvas, not docked. Active state via tonal shift + signal color icon. |
| **Status Bar** | Bottom, full width | System status + OKSG tool health (see Section 5.4) |
| **Node Inspector** | Floating, right | 280px panel, floats over canvas on `/topology` when entity selected. Not docked to edge. |

### Routes

| Route | View | Description |
|-------|------|-------------|
| `/` | Cluster Overview | Landing page, cluster health at a glance |
| `/topology` | Topology Graph | Interactive graph canvas + Node Inspector panel |
| `/metrics` | Metrics Explorer | Per-entity time-series exploration |

---

## 5. Views

### 5.1 Cluster Overview (`/`)

The landing page. Operators see cluster health at a glance. TCC reviewers see proof the discovery and graph pipeline is working.

**Layout:**
- Header row: cluster name + last discovery timestamp
- KPI row: 5 cards — nodes, namespaces, workloads, services, graph edges
- Two-column body:
  - Left: Entity Breakdown (horizontal bars per kind with counts)
  - Right: Relation Types (list of all 7 relations with counts and descriptions)
- Bottom row: Discovery Sources (chips: Prometheus, kubeletstats, K8s API) + Graph Snapshot metadata (created age, window, version)

**Data sources:**
- `GET /api/v1/discovery` → KPI counts, discovery sources
- `GET /api/v1/graph` → edge counts by relation type, snapshot metadata
- `GET /api/v1/entities?kind=...` → entity breakdown counts

**Interactions:**
- Clicking an entity kind or relation type navigates to `/topology` with that filter pre-applied

### 5.2 Topology Graph (`/topology`)

The core view. Interactive system graph rendered with React Flow. Layout is
selected per projection mode through `LAYOUT_REGISTRY`
(`web/src/views/TopologyGraph/layouts/registry.ts`): Ownership and
Infrastructure use synchronous strategies (custom tree / `@dagrejs/dagre`),
Traffic uses ELK layered orthogonal (`elkjs`). The registry pairs each mode
with a default algorithm and accepts additional algorithms keyed by name, so
new strategies plug in without touching call sites. The full graph is too
dense to be useful flat, so the view is filtered through one of three
**projection modes** with three **granularity** levels. Both are URL-backed via
`useProjectionState`, so links into the topology view round-trip mode and
granularity.

**Projection modes** (`web/src/views/TopologyGraph/projection.ts` →
`LAYER_SPECS`):

| Mode | Visible kinds | Layer-relevant relations | Layout strategy |
|------|---------------|-------------------------|-----------------|
| Ownership | namespace, workload, pod, container, configmap, secret, pvc, service, ingress | `contains`, `mounts`, `uses`, `exposes`, `routes_to` | top-down hierarchy, namespace groups side-by-side, dynamic ranks via `runLayered` |
| Infrastructure | node, pod, container | `runs_on`, `contains` | dagre LR for the single-tier cases (Nodes / Pods on nodes); hand-packed two-tier nesting for Containers in pods |
| Traffic | varies by flow — see Traffic flows below | `calls`, `routes_to`, `exposes` | ELK layered (`RIGHT`) with orthogonal edge routing; route geometry returned by ELK is rendered as-is |

**Granularity** (`Granularity = 'overview' | 'runtime' | 'detailed'`) is shared
URL state across all modes; the picker labels swap per mode via
`granularityEntriesFor(mode)` while the underlying URL value stays stable
for shareable links.

For Ownership:

- `overview` — namespace, node, service, workload, ingress
- `runtime` — overview + pod
- `detailed` — runtime + container, configmap, secret, pvc

For Infrastructure (`INFRASTRUCTURE_GRANULARITY_ENTRIES` +
`INFRASTRUCTURE_VISIBLE_KINDS`):

| URL value  | Picker label          | Visible kinds        |
|------------|-----------------------|----------------------|
| `overview` | "Nodes"               | node                 |
| `runtime`  | "Pods on nodes"       | node, pod            |
| `detailed` | "Containers in pods"  | node, pod, container |

Infrastructure is purely "where compute runs": Node ⊃ Pod ⊃ Container.
PVCs / Secrets / ConfigMaps live outside this view. At `detailed`
granularity the layout switches to a hand-packed two-tier nesting (node
rectangles contain pod rectangles contain container cards) and emits no
edges — containment is encoded by visual nesting.

**Traffic flows** (Traffic mode reuses the `Granularity` URL state but
relabels the picker — see `TRAFFIC_FLOW_SPECS` and
`TRAFFIC_GRANULARITY_ENTRIES`):

| Granularity | Flow | Visible kinds | Relations |
|-------------|------|---------------|-----------|
| `overview`  | Endpoint Flow | ingress, service | `routes_to`, `calls` |
| `runtime`   | Workload Flow | ingress, service, workload | `calls`, `exposes`, `routes_to` |
| `detailed`  | Instance Flow | pod | `calls` |

Endpoint Flow is the default and answers "which entrypoints and services
are talking?". Workload Flow bridges runtime traffic with the controllers
that implement the endpoints — `exposes` is rendered structurally (thin,
muted, dashed) so it reads distinct from the rate-styled `calls` edges.
Instance Flow renders all participating pods for replica-level
troubleshooting; pod auto-collapse and zero-traffic hiding are deferred.

**Edge filtering invariant.** The `project()` function filters edges by
**both** endpoint visibility and `LayerSpec.relations`. Without the second
filter, removing a relation from a layer's set only changes which entities
count as participants — edges of that relation still render between two
already-visible nodes (the historic `calls`-leak-into-Ownership regression).

**Canvas chrome:**
- Full-bleed graph canvas with **dotted grid background** providing spatial reference during pan/zoom
- Floating top bar: projection-mode tabs, granularity tabs, an "include orphans" toggle
- Floating `LegendPanel` (right): kind / relation legend, plus namespace color swatches in Infrastructure mode
- Minimap (bottom-left), zoom controls (bottom-right)
- Floating `NodeInspector` panel (right, conditional — appears on node click)

**Node Cards (React Flow custom nodes):**
Each entity renders as a card with a colored header and property rows.

| Color | Entities | Condition |
|-------|----------|-----------|
| Signal Green `#08dc7d` | Service, Workload, Pod | Nominal state |
| Command Red `#c62d2f` | Any entity | Error metrics present or critical labels |
| Data Blue `#3e9adf` | Node, Namespace | Infrastructure entities |
| Muted Gray `#333537` | ConfigMap, Secret, PVC | Supporting context (rendered smaller, lower opacity) |

Card content: entity name (bold, header bar), kind-specific icon (top-right), 2-3 key properties (label + value rows), and **minimalist segmented bar indicators** for metrics like CPU, MEM, I/O — solid rectangular blocks with gaps, matching the reference mockup style (e.g., the IO `450MB/s` segmented bar in the POD_COMPUTE_02 card).

**Edge Rendering — `RelationEdge`:**
Edges are a custom React Flow edge component (`edges/RelationEdge.tsx`):

- Path geometry uses React Flow's live source/target handle coordinates as the source of truth — never stale layout-box estimates. This is enforced because dagre/tree layout positions are advisory; React Flow may reposition handles after re-render and the edge must follow. **Exception:** edges with `data.rawElk === true` carry an explicit `routePoints` array produced by ELK; these are rendered as-is and the live handle coordinates are intentionally ignored, otherwise the first/last segment would tilt off the orthogonal route.
- Stroke color is keyed to relation type via CSS custom properties (`--color-signal` for `calls`, `--color-info` for `runs_on`/`contains`, `--color-text-muted` for `exposes` and other structural relations). In Workload Flow the `exposes` edge is rendered thin, muted and dashed so it visually separates from rate-styled `calls`.
- For `calls` edges, stroke width scales with the live request rate snapshot from `useEdgeMetrics`, so high-traffic links read as visually heavier.
- The original "cable bundle" mockup is approximated with stroke-width modulation rather than literal multi-line bundles; this kept the SVG cost flat as graph density grew.
- An `EdgeMetricButton` overlay opens the Metrics Explorer pre-filtered to that edge.

**Node Inspector Panel:**
Appears when a node is clicked. Contents:
- Entity name + status indicator dot. The dot reads `node.status` directly — one of the public health-status string set (`healthy | running_not_ready | restarting | pending | succeeded | failed | unknown`) supplied by the backend at the API boundary. The frontend does no enum translation; see `pkg/core/metrics/health_status.go`.
- Kind/type chips
- Properties section: namespace, kind-specific fields from entity labels
- Inbound relations: list with relation type + source entity name (clickable)
- Outbound relations: list with relation type + target entity name (clickable)
- Edge metric shortcuts on `calls` relations: navigate directly to edge metrics in the Metrics Explorer
- "View Full Metrics →" link: navigates to `/metrics?entity={id}`

**Data sources:**
- `GET /api/v1/graph` → full topology snapshot (nodes + edges)
- `GET /api/v1/graph/nodes/{id}` → selected node detail with inbound/outbound edges
- `GET /api/v1/graph/nodes?kind=&namespace=` → filtered node queries
- `GET /api/v1/graph/edges?relation=` → filtered edge queries
- `GET /api/v1/metrics?entity_id[]={id}` → metrics preview for inspector

**Interactions:**
- Pan, zoom, scroll on canvas
- Click node → open inspector
- Click related entity name in inspector → select + center that node
- Click "View Full Metrics →" → navigate to `/metrics?entity={id}`
- Filter dropdowns apply to both nodes and edges

### 5.3 Metrics Explorer (`/metrics`)

Entity and edge time-series exploration across the canonical metric categories.

Single-entity (or single-edge) workflow. Multi-entity comparison was prototyped and removed in favour of a tighter selection UX with a global keyboard search.

**Layout (entity mode):**
1. Header: page title + Entity / Edge mode tabs
2. **Global entity search** (`filter/GlobalEntitySearch.tsx`): full-width input with a `⌘K` shortcut. Fuzzy text search across name, namespace, kind, and status against the latest snapshot. Arrow keys + Enter pick.
3. **Breadcrumb / Builder filter** (`filter/FilterBreadcrumbBuilder.tsx`): path-style `Namespace / Kind / Entity` segments backed by `SearchablePopover`. An "Edit Filters" toggle expands an advanced builder with `SCOPE` (namespace, kind), `WHERE` (status, name-contains query), and `SELECT` (entity) rows. "Reset" clears all state. Each picker is fed by `applyFilters(allNodes, ...)` minus its own field, so the namespace, kind, and entity dropdowns only ever list options that are still viable under the *other* active filters; if a previously-selected entity falls out of the new candidate set, it auto-clears. Popovers dismiss on outside click or `Escape` (`useClickOutside` hook).
4. **Selection summary** (`SelectionSummary.tsx`): card with health dot, entity name, kind badge, namespace/cluster/status chips, and the segmented time-range control (`5m / 15m / 1h / 6h / 24h`).
5. **Category tabs**: derived from catalog metrics whose `scopes` and `applies_to` match the current selection.
6. **Chart grid**: one `MetricCard` per applicable canonical metric. Cards share a Recharts `syncId` so the hover cursor mirrors across cards even though only a single entity is selected.
7. **Raw Samples table**: scrollable, scope column shows `namespace/name` for entity samples and `src → tgt` for edge samples.

**Layout (edge mode):**
1. Same header, mode tab on Edge.
2. **Edge selector** (`edge/EdgeSelector.tsx`): single search-and-pick combobox over `calls` edges in the snapshot; shows source and target health dots, kind swatches, names, and namespaces. Keyboard navigable.
3. **Selection summary** in edge variant: source → target with relation badge.
4. Tabs are fixed to traffic, latency, error (the categories defined for `MetricScope = 'edge'`).
5. Same chart grid + raw samples.

**URL state:**
- Entity: `/metrics?mode=entity&entity={id}` (single id; the multi-id `entity=a,b,c` shape was retired with multi-compare).
- Edge: `/metrics?mode=edge&edgeSource={id}&edgeTarget={id}` (unchanged).
- Deep-links from the topology `NodeInspector` (`entityMetricsUrl` / `edgeMetricsUrl` in `web/src/lib/links.ts`) are preserved.

**Chart Cards (`MetricCard.tsx`):**
- Title + `InfoTooltip` showing sample count, canonical metric name, and source list (active source highlighted in Signal Green).
- Source chips listing `metric.sources[]` with the active one accent-coloured.
- Prominent current value (last sample) + unit, right-aligned.
- Recharts `BarChart` with horizontal gridlines, hidden Y axis, and a 5-tick X axis (left/center/right + "now" label on the rightmost tick).
- Empty state: dashed border + "No samples in selected time range" + the list of expected sources, so empty cards distinguish "no data right now" from "this never applies".

**Time Range:**
Segmented control inside the selection summary. Maps to `from` / `to` query params on the metrics API.

**Category Tabs:**
Maps 1:1 to the canonical `MetricCategory` values: resource, traffic, latency, error, capacity, health. Active tab highlighted in Signal Green.

**Raw Samples Table (`RawSamplesTable.tsx`):**
Scrollable table with columns: Metric, Scope, Source, (Mesh — only when mesh-labelled samples are present), Value, Unit, Timestamp. The page passes a snapshot-aware `scopeFor` formatter so users see human-readable namespace/name, not raw IDs.

**Data sources:**
- `GET /api/v1/metrics/series?metric_name={name}&entity_id[]={id}&from=&to=` → entity metric series
- `GET /api/v1/metrics/series?metric_name={name}&edge_source_id={id}&edge_target_id={id}&from=&to=` → edge metric series
- `GET /api/v1/metrics/catalog?category=` → canonical metric definitions, including per-metric `scopes` and `applies_to`
- `GET /api/v1/graph` → graph edges for the edge selector
- `GET /api/v1/graph` → live graph nodes for the global search and breadcrumb filter

The Metrics Explorer must treat catalog `scopes` as authoritative:

- entity mode renders only metrics whose scopes include `entity`
- edge mode renders only metrics whose scopes include `edge`
- edge mode uses edge-capable catalog entries such as servicegraph and HTTP request metrics; unsupported pod-network metrics must not be requested or displayed

**Applicability layer.** On top of `scopes`, the Explorer hides tabs and cards that are *structurally* inapplicable to the selected entity kind using catalog-owned `applies_to` metadata. For workload entities, `k8s.workload.type` narrows controller-specific metric families so Deployment, StatefulSet, DaemonSet, ReplicaSet, Job, CronJob, and ReplicationController cards only appear on the matching subtype; missing subtype metadata fails open for older graph snapshots. `sources` describe metric producers and source arbitration only; the frontend must not infer entity applicability from source IDs.

Cards that are applicable but currently empty render an informative empty state listing the metric's expected sources, distinguishing "no data right now" from "this never applies". Catalog responses missing `applies_to` fail open for compatibility and emit a single `console.warn` in dev. See `web/src/views/MetricsExplorer/applicability/README.md` for the full model and how to extend it.

### 5.4 Status Bar

Persistent bottom bar across all routes. Shows both cluster system status and OKSG tool health.

**Layout:** Single horizontal bar, left-to-right:
- **System status indicator**: green dot + `SYSTEM_NOMINAL` (or red + `DEGRADED` based on `/api/v1/status` components)
- **OKSG tool status**: `[OKSG] BACKEND CONNECTED · LAST SNAPSHOT 12s AGO · 52 NODES · 56 EDGES`
- **Network info** (right-aligned): `[NET] POLLING oksg-backend:8080`

**Status logic:**
- `SYSTEM_NOMINAL`: all components in `/api/v1/status` report healthy
- `DEGRADED`: one or more components unhealthy
- `DISCONNECTED`: backend unreachable (fetch fails)
- Last snapshot age: computed from `GET /api/v1/graph` → `created_at` vs now
- Node/edge counts: from the latest graph snapshot

**Data source:** `GET /api/v1/status` polled every 30s via `useSystemStatus` hook.

---

## 6. Data Layer Architecture

### Principle: Polling Now, WebSocket-Ready

The backend has REST-only endpoints. Topology snapshots flush every 60s. The frontend polls on intervals. The data layer is structured so a WebSocket upgrade later is a transport swap, not a rewrite.

### HTTP Client: Native `fetch`

No axios. The API client is a thin wrapper around the browser's native `fetch`:
- `client.ts` provides a `request<T>(path, options)` function that handles base URL, JSON parsing, error mapping, and typed responses
- Each API domain module (`graph.ts`, `metrics.ts`, etc.) exports pure functions that call `request<T>`
- TanStack Query's `queryFn` calls these functions — components never call `fetch` directly

### Directory Structure (modular by domain)

```
src/
  api/
    client.ts          — thin native fetch wrapper (base URL, JSON, error handling)
    graph.ts           — getSnapshot(), getNode(id), getEdges(filters)
    metrics.ts         — getSamples(entityId, timeRange), getCatalog()
    entities.ts        — listEntities(filters), getEntity(id)
    discovery.ts       — getDiscovery(), getStatus()
  hooks/
    useGraphSnapshot.ts   — TanStack Query + polling (refetchInterval: 30s)
    useNodeDetail.ts      — fetches node + edges on entity selection
    useMetrics.ts         — fetches samples with time range and category
    useSystemStatus.ts    — polls /status for status bar, derives connection state
    useDiscovery.ts       — polls /discovery for cluster overview
  stores/
    useAppStore.ts        — Zustand: selectedEntityId, activeFilters, connectionStatus
  types/
    graph.ts              — Snapshot, Node, Edge, EntityKind, RelationType
    metrics.ts            — MetricSample, MetricCategory, CanonicalMetric
    entities.ts           — Entity, EntityQuery
    api.ts                — API response envelopes, error types
  styles/
    themes/
      dark.css           — dark theme variables ([data-theme="dark"])
      light.css          — light theme variables (future, same var names)
    tokens.css           — semantic aliases, spacing scale, typography tokens
    global.css           — base styles, font import, theme loading
```

Each layer is independently replaceable: swap `api/client.ts` to change HTTP transport, swap `themes/dark.css` to change visual theme, swap a `hooks/` file to change data fetching strategy.

### WebSocket-Ready Pattern

TanStack Query hooks are the **only** data consumers. Components never fetch directly. To upgrade to WebSocket:

1. Add a WS connection manager
2. On WS message, call `queryClient.setQueryData('graph', newSnapshot)`
3. Remove `refetchInterval` from the graph query
4. Zero component changes needed

### Polling Intervals

| Query | Interval | Endpoint |
|-------|----------|----------|
| Graph snapshot | 30s | `GET /api/v1/graph` |
| System status | 30s | `GET /api/v1/status` |
| Discovery | 120s | `GET /api/v1/discovery` |
| Metrics (when visible) | 30s | `GET /api/v1/metrics` |
| Node detail | On demand | `GET /api/v1/graph/nodes/{id}` |
| Metric catalog | Once (stale: Infinity) | `GET /api/v1/metrics/catalog` |

---

## 7. Deployment

**The frontend always runs inside a Docker container** — both in development and production. There is no local/bare-metal execution path.

### Development

- A `Dockerfile.dev` provides a development container with Vite's dev server (HMR enabled)
- `docker-compose.yml` (or a `task dev` command) spins up the frontend container alongside the backend
- Vite dev server runs inside the container, with source files mounted as a volume for live reload
- API requests proxy to the backend container via Vite's `server.proxy` config
- Developers interact with the app via `http://localhost:<mapped-port>` but the process itself runs in Docker

### Production (Kubernetes)

- **Build:** Multi-stage Dockerfile — stage 1 builds static files with Vite, stage 2 copies `dist/` into an nginx image
- **Container:** nginx serving static files + reverse proxy for `/api/*` → `oksg-backend:8080`
- **Helm:** Deployed as additional templates under `deploy/helm/oksg-otel/`
  - Deployment: single replica, nginx image with built frontend baked in
  - Service: ClusterIP on port 80
  - ConfigMap: nginx.conf with `/api/*` proxy pass to `oksg-backend:8080`
- **Service name:** `oksg-frontend` in `oksg-system` namespace
- **No direct backend access from the browser** — nginx handles the API proxy

---

## 8. Project Structure

```
web/
  package.json
  tsconfig.json
  vite.config.ts
  index.html
  public/
  src/
    main.tsx               — app entry, providers (QueryClient, Router, theme init)
    App.tsx                — shell layout with floating sidebar, top bar, status bar, <Outlet/>
    api/
      client.ts            — thin native fetch wrapper
      graph.ts             — graph API functions
      metrics.ts           — metrics API functions
      entities.ts          — entity API functions
      discovery.ts         — discovery/status API functions
    hooks/                 — TanStack Query + Zustand hooks (one per data domain)
    stores/                — Zustand stores
    types/                 — TypeScript types matching backend API contracts
    views/
      ClusterOverview/     — / route components
      TopologyGraph/       — /topology route components
        nodes/             — React Flow custom node components per entity kind
        edges/             — React Flow custom edge component (cable bundle renderer)
        NodeInspector.tsx  — floating right panel
        FilterToolbar.tsx  — floating filter chips
      MetricsExplorer/     — /metrics route components
    components/
      Shell/               — TopBar (floating), Sidebar (floating), StatusBar
      shared/              — reusable primitives (DataChip, SegmentedBar, etc.)
    styles/
      themes/
        dark.css           — [data-theme="dark"] CSS custom property definitions
        light.css          — [data-theme="light"] (future, same variable names)
      tokens.css           — semantic aliases, spacing scale, typography tokens
      global.css           — base styles, font import, dotted canvas background
```

---

## 9. Key Libraries

| Package | Version | Purpose |
|---------|---------|---------|
| `react` | ^18 | UI framework |
| `react-dom` | ^18 | DOM rendering |
| `react-router-dom` | ^6 | Client-side routing |
| `@xyflow/react` | ^12 | Graph visualization (React Flow) |
| `recharts` | ^2 | Time-series charts |
| `zustand` | ^4 | State management |
| `@tanstack/react-query` | ^5 | Data fetching with polling |
| `lucide-react` | ^0.468 | Consistent geometric icon set with adjustable stroke width |
| `typescript` | ^5 | Type safety |

No additional UI frameworks or CSS frameworks.

---

## 10. Working Rules

Constraints that have caused regressions if ignored:

- **Layout strategies are synchronous.** `useGraphLayout` is a `useMemo`
  dispatcher keyed by `ProjectionMode`; do not convert it to `useEffect` +
  `setState`. The `LayoutStrategy` interface in `layouts/types.ts` is the
  contract every strategy implements.
- **Layouts are picked through `LAYOUT_REGISTRY`** in
  `web/src/views/TopologyGraph/layouts/registry.ts`. Ownership runs a custom
  tree layout, Infrastructure uses `@dagrejs/dagre`, Traffic uses ELK
  (`elkjs`) layered orthogonal. The registry maps each `ProjectionMode` to a
  default algorithm plus any optional alternates so adding a new strategy is
  a one-line change.
- **Catalog scopes are authoritative.** Metrics Explorer renders entity
  cards only when the catalog entry's `scopes` includes `entity`, and edge
  cards only when it includes `edge`.
- **Health summary.** Cluster Overview's runtime health row reads
  `/api/v1/status.health_summary`: node health from
  `k8s.node.condition.ready`, pod health from the latest topology snapshot's
  pod labels (`phase`, `status`, `k8s.pod.phase`) with an explicit `unknown`
  bucket. The UI does not infer phase from absence of telemetry.
- **Node Inspector** renders only live metric labels attached to graph
  nodes (CPU, memory, phase, etc.) plus inbound/outbound relations and the
  "View Full Metrics →" link. No fabricated bars or sparklines.
- **Status bar.** `BACKEND_OK` / `BACKEND_OFFLINE` / `DISCONNECTED` are
  derived from the `/api/v1/status` poll and fetch-failure fallback.
  Topology screenshots are only valid evidence when the footer reads
  `BACKEND_OK`.
- **Topology projection layer relations** (`projection.ts → LAYER_SPECS`):
  - Ownership relations must include `contains`, `mounts`, `uses`,
    `exposes`, and `routes_to`. Removing any one of them filters Ingress
    or call-only services out of `computeParticipants` and they vanish.
  - Do **NOT** add `calls` to ownership; `calls` belongs to Traffic only.
  - Traffic relations: `calls`, `routes_to`, `exposes`. Infrastructure:
    `runs_on`, `mounts`. Don't blend them.
  - The edge filter in `project()` MUST check
    `spec.relations.has(e.relation)` in addition to endpoint visibility,
    or stale relations leak into the wrong layer.
