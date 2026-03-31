# Frontend Design Specification — OKSG Visualizer

**Date:** 2026-03-30
**Layer:** 6B — Control, Query, and Visualization (UI)
**Status:** Approved design, pending implementation

---

## 1. Purpose

The OKSG frontend is a read-only observability dashboard that renders the topology graph and metrics produced by the OKSG backend. It serves two audiences:

- **Primary — Operators/SREs:** Monitor a live Kubernetes cluster, spot issues quickly, navigate the graph to understand dependencies and locate root causes.
- **Secondary — TCC/academic reviewers:** Evaluate the architecture by seeing the system graph and metrics foundation in action as proof the observability-to-graph pipeline works end to end.

The frontend is purely observational. No cluster mutations, no control-plane operations.

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
| Data Fetching | TanStack Query | Polling intervals, caching, stale detection, retry |
| Styling | CSS Modules | Aligns with flat-hex design tokens, no framework fighting the aesthetic |
| Font | Space Grotesk (Google Fonts) | Per design system spec |
| Language | TypeScript | Type safety against API contracts |

**No UI framework** (no MUI, Chakra, etc.). The "Graphic Retro-Futurism" design system is custom — flat surfaces, tonal layering, zero shadows/gradients/borders.

---

## 3. Design System Reference

The visual language follows the "Graphic Retro-Futurism / The Analog Superintendent" design system. Key rules:

### Colors (Dark Mode only)
- **Signal Green** `#08dc7d` — nominal state, primary actions
- **Command Red** `#c62d2f` — critical failures, error state
- **Data Blue** `#3e9adf` — informational, infrastructure entities
- **Surface Dark** `#1b1d1f` — application background
- **Text Muted** `#a3a7af` — secondary metadata, labels

### Surface Hierarchy (tonal layering, no borders)
1. Base: `#121416`
2. Navigation/Sidebar: `#1a1c1e`
3. Main modules: `#1e2022`
4. Active/Hovered: `#333537`

### Typography
- **Font:** Space Grotesk exclusively
- **Headlines:** 1.5rem, tight letter-spacing (-0.02em)
- **Labels:** 0.6875rem, uppercase, wide letter-spacing (0.08em)
- **Body/Data:** 0.75rem

### Rules
- No 1px borders — boundaries via background color shifts only
- No shadows — hierarchy via tonal layering
- No gradients — every element is a flat hex code
- Corner radius: 12-16px for cards/modules
- Geometric/wireframe icons matching Space Grotesk stroke weights

---

## 4. Architecture

### Pattern: Route-Based Views with Shared Data Layer

Three routes sharing a persistent shell (sidebar + top bar + status bar). Zustand for cross-view state. TanStack Query for data fetching with polling.

### Application Shell (persistent across all routes)

| Component | Position | Description |
|-----------|----------|-------------|
| **Top Bar** | Top, full width | OKSG branding, polling interval indicator, connection status dot |
| **Sidebar** | Left, 56px | Icon navigation for 3 routes. Active state via tonal shift + green icon |
| **Status Bar** | Bottom, full width | System status + OKSG tool health (see Section 5.4) |

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

The core view. Interactive system graph rendered with React Flow.

**Layout:**
- Full-bleed graph canvas with:
  - Filter toolbar (top): Kind, Namespace, Relation type dropdowns
  - Minimap (bottom-left)
  - Zoom controls (bottom-right)
- Node Inspector panel (right, 280px, conditional — appears on node click)

**Node Cards (React Flow custom nodes):**
Each entity renders as a card with a colored header and property rows.

| Color | Entities | Condition |
|-------|----------|-----------|
| Signal Green `#08dc7d` | Service, Workload, Pod | Nominal state |
| Command Red `#c62d2f` | Any entity | Error metrics present or critical labels |
| Data Blue `#3e9adf` | Node, Namespace | Infrastructure entities |
| Muted Gray `#333537` | ConfigMap, Secret, PVC | Supporting context (rendered smaller, lower opacity) |

Card content: entity name (header), kind-specific icon, 2-3 key properties (kind, namespace, replicas, type, etc.).

**Edge Rendering:**
- Labeled lines showing relation type
- Color-matched: green for calls/exposes, blue for runs_on, muted for uses/mounts
- Labels on hover or always visible (configurable)

**Node Inspector Panel:**
Appears when a node is clicked. Contents:
- Entity name + status indicator dot
- Kind/type chips
- Properties section: namespace, kind-specific fields from entity labels
- Inbound relations: list with relation type + source entity name (clickable)
- Outbound relations: list with relation type + target entity name (clickable)
- Metrics preview: CPU and MEM bars with current values
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

Per-entity time-series exploration across the 5 canonical metric categories.

**Layout:**
- Header: entity name + Entity selector dropdown + Time range dropdown
- Category tabs: Resource, Traffic, Latency, Error, Capacity
- Chart grid (2 columns): one card per metric in the active category
- Raw Samples table: scrollable table of MetricSample records

**Entity Selector:**
Dropdown listing all known entities. Pre-filled when arriving from the topology inspector via `/metrics?entity={id}`.

**Time Range:**
Dropdown: Last 5m, 15m, 1h, 6h, 24h. Maps to `from` and `to` query params on the metrics API.

**Category Tabs:**
Maps 1:1 to the 5 canonical MetricCategories: resource, traffic, latency, error, capacity. Active tab highlighted in Signal Green.

**Chart Cards:**
Each canonical metric for the selected entity gets a card containing:
- Metric name (uppercase label)
- Current value + unit (large, colored)
- Time-series chart: line charts for continuous metrics, bar charts for throughput
- Multi-series support where applicable (e.g., disk read/write on same chart)

**Raw Samples Table:**
Scrollable table with columns: Metric, Value, Unit, Timestamp. Zebra-striped with tonal layering (alternating `#1a1c1e` / `#1e2022`). Useful for operators debugging and TCC reviewers inspecting raw data.

**Data sources:**
- `GET /api/v1/metrics?entity_id[]={id}&from=&to=` → metric samples
- `GET /api/v1/metrics/catalog?category=` → canonical metric definitions
- `GET /api/v1/entities` → entity list for the selector

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

### Directory Structure

```
src/
  api/
    client.ts          — base fetch wrapper (base URL, headers, error handling)
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
```

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

- **Build output:** Vite produces static files (`dist/`)
- **Container:** nginx serving static files + reverse proxy for `/api/*` → `oksg-backend:8080`
- **Helm:** Deployed as a subchart or additional templates under `deploy/helm/oksg-otel/`
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
    main.tsx               — app entry, providers (QueryClient, Router)
    App.tsx                — shell layout with sidebar, top bar, status bar, <Outlet/>
    api/                   — typed fetch functions per API domain
    hooks/                 — TanStack Query + Zustand hooks
    stores/                — Zustand stores
    types/                 — TypeScript types matching backend API contracts
    views/
      ClusterOverview/     — / route components
      TopologyGraph/       — /topology route components
        nodes/             — React Flow custom node components per entity kind
        edges/             — React Flow custom edge components
        NodeInspector.tsx  — right panel
        FilterToolbar.tsx  — filter chips
      MetricsExplorer/     — /metrics route components
    components/
      Shell/               — TopBar, Sidebar, StatusBar
      shared/              — reusable primitives (DataChip, MetricBar, etc.)
    styles/
      tokens.css           — design system color/spacing/typography tokens
      global.css           — base styles, font import
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
| `typescript` | ^5 | Type safety |

No additional UI frameworks, icon libraries (custom geometric SVGs), or CSS frameworks.
