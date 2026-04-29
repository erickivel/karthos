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

The core view. Interactive system graph rendered with React Flow.

**Layout:**
- Full-bleed graph canvas with **dotted grid background** (subtle dots at regular intervals using `var(--surface-active)` on `var(--surface-module)`) providing spatial reference during pan/zoom
- Floating filter toolbar (top): Kind, Namespace, Relation type dropdowns
- Minimap (bottom-left)
- Zoom controls (bottom-right)
- Floating Node Inspector panel (right, 280px, conditional — appears on node click)

**Node Cards (React Flow custom nodes):**
Each entity renders as a card with a colored header and property rows.

| Color | Entities | Condition |
|-------|----------|-----------|
| Signal Green `#08dc7d` | Service, Workload, Pod | Nominal state |
| Command Red `#c62d2f` | Any entity | Error metrics present or critical labels |
| Data Blue `#3e9adf` | Node, Namespace | Infrastructure entities |
| Muted Gray `#333537` | ConfigMap, Secret, PVC | Supporting context (rendered smaller, lower opacity) |

Card content: entity name (bold, header bar), kind-specific icon (top-right), 2-3 key properties (label + value rows), and **minimalist segmented bar indicators** for metrics like CPU, MEM, I/O — solid rectangular blocks with gaps, matching the reference mockup style (e.g., the IO `450MB/s` segmented bar in the POD_COMPUTE_02 card).

**Edge Rendering — "Cable Bundle" Style:**
Edges are rendered as organized cable bundles, inspired by physical network cluster cabling:
- Each edge is a bundle of parallel lines ("cables") routed with right-angle or smooth orthogonal paths
- **Cable count and thickness scale with traffic/confidence:** high-traffic `calls` edges render as thick multi-cable bundles; low-traffic `uses` edges render as thin single cables
- Color-matched to relation type: `var(--color-signal)` for calls/exposes, `var(--color-info)` for runs_on, `var(--color-text-muted)` for uses/mounts
- Cables route along grid-aligned paths (horizontal → vertical → horizontal) to maintain the organized "server rack wiring" aesthetic
- Relation type label shown on hover or always visible (configurable)
- Custom React Flow edge component handles the multi-cable rendering and traffic-based scaling

**Node Inspector Panel:**
Appears when a node is clicked. Contents:
- Entity name + status indicator dot
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

**Layout:**
- Header: mode switch + Entity or Edge selector + Time range dropdown
- Category tabs: Resource, Traffic, Latency, Error, Capacity, Health
- Chart grid (2 columns): one card per metric in the active category
- Raw Samples table: scrollable table of MetricSample records

**Entity mode:**
Dropdown listing entities from the latest graph snapshot only. Pre-filled when arriving from the topology inspector via `/metrics?entity={id}`.

**Edge mode:**
Dropdown listing graph `calls` edges by source and target display names. Pre-filled when arriving from the topology inspector via `/metrics?mode=edge&edgeSource={id}&edgeTarget={id}`.

**Time Range:**
Dropdown: Last 5m, 15m, 1h, 6h, 24h. Maps to `from` and `to` query params on the metrics API.

**Category Tabs:**
Maps 1:1 to the canonical `MetricCategory` values: resource, traffic, latency, error, capacity, health. Active tab highlighted in Signal Green.

**Chart Cards:**
Each canonical metric for the selected entity gets a card containing:
- Metric name (uppercase label)
- Current value + unit (large, colored)
- Time-series chart: **minimalist segmented bar charts** as the primary visualization style — solid rectangular blocks with gaps between them, using opacity gradients from `var(--surface-active)` (older data) to `var(--color-signal)` (recent data), matching the "Live Telemetry" aesthetic from the reference mockups
- Line charts used only where continuous trend is essential (e.g., latency percentiles)
- Multi-series support where applicable (e.g., disk read/write on same chart)

**Raw Samples Table:**
Scrollable table with columns: Metric, Value, Unit, Timestamp. Zebra-striped with tonal layering (alternating `var(--surface-nav)` / `var(--surface-module)`). Useful for operators debugging and TCC reviewers inspecting raw data.

**Data sources:**
- `GET /api/v1/metrics/series?metric_name={name}&entity_id[]={id}&from=&to=` → entity metric series
- `GET /api/v1/metrics/series?metric_name={name}&edge_source_id={id}&edge_target_id={id}&from=&to=` → edge metric series
- `GET /api/v1/metrics/catalog?category=` → canonical metric definitions, including per-metric `scopes`
- `GET /api/v1/graph` → graph edges for the edge selector
- `GET /api/v1/graph` → live graph nodes for the entity selector

The Metrics Explorer must treat catalog `scopes` as authoritative:

- entity mode renders only metrics whose scopes include `entity`
- edge mode renders only metrics whose scopes include `edge`
- edge mode uses edge-capable catalog entries such as servicegraph and HTTP request metrics; unsupported pod-network metrics must not be requested or displayed

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
