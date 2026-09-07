// Topology projection: turns the raw graph into a per-mode subgraph.
//
// LOAD-BEARING DECISIONS — DO NOT REVERT WITHOUT READING THESE FIRST.
//
// 1. Ownership relations include the structural wiring relations
//    (`exposes`, `routes_to`) on top of strictly structural ones
//    (`contains`, `mounts`, `uses`). If you narrow ownership by removing
//    any of those five, Ingress / call-only services / similar entities
//    lose every layer-relevant edge, drop out of `computeParticipants`,
//    and disappear from the view. Do NOT add `calls` here — that's a
//    runtime traffic relation and belongs to the Traffic layer only.
//
// 2. Layer `relations` decides which edges render between visible entities.
//    It also determines structural context for non-traffic layers. Traffic
//    starts from observed `calls` evidence, then expands through its allowed
//    structural context. This keeps idle workload wiring out of a live
//    traffic flow.
//
// 3. Traffic mode is split across THREE flows by granularity:
//    - overview  → Endpoint Flow  (ingress, service; routes_to + calls)
//    - runtime   → Workload Flow  (ingress, service, workload; calls + exposes + routes_to)
//    - detailed  → Instance Flow  (pod; calls)
//    `resolveLayerSpec` picks the right LayerSpec per (mode, granularity).
//    Endpoint and Instance Flow MUST NOT include `exposes` — exposes is
//    structural and only appears in Workload Flow. Re-introducing the old
//    `mode === 'traffic'` overview override would collapse Workload and
//    Instance Flows back to Endpoint Flow.
//
import type {
  EntityKind,
  GraphEdge,
  GraphNode,
  RelationType,
} from '@/types/graph'

export type ProjectionMode = 'ownership' | 'infrastructure' | 'traffic'
export type Granularity = 'overview' | 'runtime' | 'detailed'

const OVERVIEW_KINDS: EntityKind[] = [
  'namespace',
  'node',
  'service',
  'workload',
  'ingress',
]
const RUNTIME_ADDS: EntityKind[] = ['pod']
const DETAILED_ADDS: EntityKind[] = ['container', 'configmap', 'secret', 'pvc']

export const VISIBLE_KINDS_BY_GRANULARITY: Record<Granularity, Set<EntityKind>> = {
  overview: new Set<EntityKind>(OVERVIEW_KINDS),
  runtime: new Set<EntityKind>([...OVERVIEW_KINDS, ...RUNTIME_ADDS]),
  detailed: new Set<EntityKind>([
    ...OVERVIEW_KINDS,
    ...RUNTIME_ADDS,
    ...DETAILED_ADDS,
  ]),
}

// Infrastructure has its own granularity meaning (Node / Pod / Container)
// and does not share OVERVIEW_KINDS with Ownership.
export const INFRASTRUCTURE_VISIBLE_KINDS: Record<Granularity, Set<EntityKind>> =
  {
    overview: new Set<EntityKind>(['node']),
    runtime: new Set<EntityKind>(['node', 'pod']),
    detailed: new Set<EntityKind>(['node', 'pod', 'container']),
  }

// LAYER_SPECS — single registry. Adding a new layer is one entry: kinds
// allow-list, layer-relevant relations, and an intrinsic predicate (entities
// that always participate regardless of edges, e.g. a namespace in ownership).
// A `contains` walk extends participation transitively (pods under a
// participating workload also participate).
export interface LayerSpec {
  id: ProjectionMode
  kinds: ReadonlySet<EntityKind>
  relations: ReadonlySet<RelationType>
  intrinsic?: (entity: GraphNode) => boolean
}

export const LAYER_SPECS: Record<ProjectionMode, LayerSpec> = {
  ownership: {
    id: 'ownership',
    kinds: new Set<EntityKind>([
      'namespace',
      'workload',
      'pod',
      'container',
      'configmap',
      'secret',
      'pvc',
      'service',
      'ingress',
    ]),
    // Ownership shows the Kubernetes object graph: structural relations
    // (`contains`, `mounts`, `uses`) plus the structural wiring relations
    // (`exposes`, `routes_to`) that connect Ingress→Service→Workload. Without
    // the wiring relations, Ingress / call-only services appear as orphans
    // and get filtered out by `computeParticipants`.
    //
    // DO NOT add `calls` to this set. `calls` is a runtime traffic relation
    // and belongs to the Traffic layer only. Adding it here pollutes the
    // ownership view with traffic edges that don't represent K8s ownership.
    // (Removing one of `contains`/`mounts`/`uses`/`exposes`/`routes_to`
    // would re-introduce the missing-entities regression — keep all five.)
    relations: new Set<RelationType>([
      'contains',
      'mounts',
      'uses',
      'exposes',
      'routes_to',
    ]),
    // Anything with a namespace, or namespaces themselves, intrinsically
    // belongs to the ownership layer.
    intrinsic: (e) => e.kind === 'namespace' || Boolean(e.namespace),
  },
  infrastructure: {
    id: 'infrastructure',
    // Infrastructure is purely "where compute runs": Node ⊃ Pod ⊃ Container.
    // PVCs / Secrets / ConfigMaps are storage/config resources, not runtime
    // hosts, and live outside this view.
    kinds: new Set<EntityKind>(['node', 'pod', 'container']),
    // `runs_on` ties pods to their host node; `contains` ties containers to
    // their pod. Both relations are structural — they describe the K8s
    // hosting hierarchy, not observed traffic.
    relations: new Set<RelationType>(['runs_on', 'contains']),
    intrinsic: (e) => e.kind === 'node',
  },
  // Traffic at default granularity (overview) is the Endpoint Flow.
  // Workload (runtime) and Instance (detailed) flows live in
  // TRAFFIC_FLOW_SPECS below; resolveLayerSpec picks the right one.
  traffic: {
    id: 'traffic',
    kinds: new Set<EntityKind>(['service', 'ingress']),
    relations: new Set<RelationType>(['calls', 'routes_to']),
  },
}

// Traffic mode renders three distinct flows, picked by granularity.
// Endpoint Flow (overview): logical traffic surface, no workloads/pods.
// Workload Flow (runtime): which controllers implement traffic; `exposes`
// is structural here and rendered visually distinct from `calls`.
// Instance Flow (detailed): pod-level participants only.
export const TRAFFIC_FLOW_SPECS: Record<Granularity, LayerSpec> = {
  overview: {
    id: 'traffic',
    kinds: new Set<EntityKind>(['service', 'ingress']),
    relations: new Set<RelationType>(['calls', 'routes_to']),
  },
  runtime: {
    id: 'traffic',
    kinds: new Set<EntityKind>(['service', 'workload', 'ingress']),
    relations: new Set<RelationType>(['calls', 'exposes', 'routes_to']),
  },
  detailed: {
    id: 'traffic',
    kinds: new Set<EntityKind>(['pod']),
    relations: new Set<RelationType>(['calls']),
  },
}

export function resolveLayerSpec(
  mode: ProjectionMode,
  granularity: Granularity,
): LayerSpec {
  if (mode === 'traffic') return TRAFFIC_FLOW_SPECS[granularity]
  return LAYER_SPECS[mode]
}

export interface ProjectionResult {
  nodes: GraphNode[]
  edges: GraphEdge[]
  groupOf: Map<string, string>
  parentKind: EntityKind | null
}

export function visibleKindsFor(
  mode: ProjectionMode,
  granularity: Granularity,
): Set<EntityKind> {
  // Traffic is granularity-driven through TRAFFIC_FLOW_SPECS: each flow
  // declares exactly the kinds it wants (Endpoint=ingress/service,
  // Workload=+workload, Instance=pod-only). The old overview override was
  // removed deliberately — see load-bearing decision #3 at the top.
  if (mode === 'traffic') {
    return new Set(TRAFFIC_FLOW_SPECS[granularity].kinds)
  }

  if (mode === 'infrastructure') {
    return new Set(INFRASTRUCTURE_VISIBLE_KINDS[granularity])
  }

  const base = new Set(VISIBLE_KINDS_BY_GRANULARITY[granularity])
  const allowed = LAYER_SPECS[mode].kinds
  for (const kind of base) {
    if (!allowed.has(kind)) base.delete(kind)
  }
  return base
}

export function parentKindFor(mode: ProjectionMode): EntityKind | null {
  if (mode === 'ownership') return 'namespace'
  if (mode === 'infrastructure') return 'node'
  return null
}

// Comparator-driven sort. Swap the comparator to change the sort key without
// touching call sites.
export type EntityComparator = (a: GraphNode, b: GraphNode) => number
export type EdgeComparator = (a: GraphEdge, b: GraphEdge) => number

export const byEntityName: EntityComparator = (a, b) =>
  a.name.localeCompare(b.name)

export function sortEntities(
  items: GraphNode[],
  cmp: EntityComparator = byEntityName,
): GraphNode[] {
  return [...items].sort(cmp)
}

export function sortEdges(
  items: GraphEdge[],
  resolveName: (id: string) => string,
): GraphEdge[] {
  return [...items].sort((a, b) => {
    const sa = resolveName(a.source)
    const sb = resolveName(b.source)
    if (sa !== sb) return sa.localeCompare(sb)
    const ta = resolveName(a.target)
    const tb = resolveName(b.target)
    if (ta !== tb) return ta.localeCompare(tb)
    return a.relation.localeCompare(b.relation)
  })
}

// Compute the set of entity IDs that "participate" in a given layer.
// Direct participation: appears as either endpoint of a layer-relevant edge,
// or matches the intrinsic predicate. Transitive participation: any entity
// reachable via `contains` edges from a participant also participates (so
// pods under a participating workload show up in the traffic layer, etc).
function computeParticipants(
  entities: GraphNode[],
  edges: GraphEdge[],
  spec: LayerSpec,
): Set<string> {
  const ids = new Set<string>()

  for (const edge of edges) {
    if (spec.relations.has(edge.relation)) {
      ids.add(edge.source)
      ids.add(edge.target)
    }
  }

  if (spec.intrinsic) {
    for (const e of entities) {
      if (spec.intrinsic(e)) ids.add(e.id)
    }
  }

  let changed = true
  while (changed) {
    changed = false
    for (const edge of edges) {
      if (edge.relation !== 'contains') continue
      if (ids.has(edge.source) && !ids.has(edge.target)) {
        ids.add(edge.target)
        changed = true
      }
    }
  }

  return ids
}

// A traffic view is evidence-led: `calls` is the observed signal, while
// `routes_to` and (for Workload Flow) `exposes` explain how that observed
// traffic reaches Kubernetes endpoints and controllers. Treating every
// structural `exposes` edge as a seed makes all installed workloads look like
// active traffic participants even when no telemetry has observed them.
function computeTrafficParticipants(
  edges: GraphEdge[],
  spec: LayerSpec,
): Set<string> {
  const ids = new Set<string>()

  for (const edge of edges) {
    if (edge.relation === 'calls') {
      ids.add(edge.source)
      ids.add(edge.target)
    }
  }

  let changed = true
  while (changed) {
    changed = false
    for (const edge of edges) {
      if (!spec.relations.has(edge.relation)) continue
      if (!ids.has(edge.source) && !ids.has(edge.target)) continue
      if (!ids.has(edge.source)) {
        ids.add(edge.source)
        changed = true
      }
      if (!ids.has(edge.target)) {
        ids.add(edge.target)
        changed = true
      }
    }
  }

  return ids
}

export function project(
  nodes: GraphNode[],
  edges: GraphEdge[],
  mode: ProjectionMode,
  granularity: Granularity,
  options: { includeOrphans?: boolean } = {},
): ProjectionResult {
  const visibleKinds = visibleKindsFor(mode, granularity)
  const parentKind = parentKindFor(mode)
  const spec = resolveLayerSpec(mode, granularity)
  const includeOrphans = options.includeOrphans ?? false

  const kindFiltered = nodes.filter((n) => visibleKinds.has(n.kind))

  let visibleNodes: GraphNode[]
  if (includeOrphans) {
    visibleNodes = kindFiltered
  } else {
    const participants = mode === 'traffic'
      ? computeTrafficParticipants(edges, spec)
      : computeParticipants(kindFiltered, edges, spec)
    visibleNodes = kindFiltered.filter((n) => participants.has(n.id))
  }

  visibleNodes = sortEntities(visibleNodes)

  const visibleIds = new Set(visibleNodes.map((n) => n.id))
  // Both endpoints must be visible AND the relation must be in this layer's
  // relation set. The relation filter is what keeps `calls` (a Traffic-only
  // relation) out of the Ownership view even when both call-endpoint
  // services are visible via structural relations. Without it, removing
  // `calls` from `ownership.relations` only changes participant detection;
  // calls edges between already-visible services still render.
  const visibleEdgesUnsorted = edges.filter(
    (e) =>
      visibleIds.has(e.source) &&
      visibleIds.has(e.target) &&
      spec.relations.has(e.relation),
  )
  const nameById = new Map(visibleNodes.map((n) => [n.id, n.name]))
  const visibleEdges = sortEdges(visibleEdgesUnsorted, (id) => nameById.get(id) ?? id)

  const groupOf = new Map<string, string>()

  if (parentKind === 'namespace') {
    const nsByName = new Map<string, GraphNode>()
    for (const n of visibleNodes) if (n.kind === 'namespace') nsByName.set(n.name, n)
    for (const n of visibleNodes) {
      if (n.kind === 'namespace') continue
      const ns = n.namespace ? nsByName.get(n.namespace) : undefined
      if (ns) groupOf.set(n.id, ns.id)
    }
  } else if (parentKind === 'node') {
    // Infrastructure uses two-tier nesting: pods group under their host node,
    // containers group under their pod. The layout walks `groupOf` to render
    // Node ⊃ Pod ⊃ Container as nested rectangles.
    const nodeEntitiesById = new Map<string, GraphNode>()
    const podEntitiesById = new Map<string, GraphNode>()
    for (const n of visibleNodes) {
      if (n.kind === 'node') nodeEntitiesById.set(n.id, n)
      else if (n.kind === 'pod') podEntitiesById.set(n.id, n)
    }

    const podToHostNode = new Map<string, string>()
    const containerToPod = new Map<string, string>()
    for (const e of edges) {
      if (e.relation === 'runs_on') {
        if (nodeEntitiesById.has(e.target)) podToHostNode.set(e.source, e.target)
      } else if (e.relation === 'contains') {
        containerToPod.set(e.target, e.source)
      }
    }
    for (const n of visibleNodes) {
      if (n.kind === 'pod') {
        const host = podToHostNode.get(n.id)
        if (host) groupOf.set(n.id, host)
      } else if (n.kind === 'container') {
        const podId = containerToPod.get(n.id)
        if (podId && podEntitiesById.has(podId)) {
          groupOf.set(n.id, podId)
        }
      }
    }
  }

  return { nodes: visibleNodes, edges: visibleEdges, groupOf, parentKind }
}

export function isProjectionMode(v: string | null | undefined): v is ProjectionMode {
  return v === 'ownership' || v === 'infrastructure' || v === 'traffic'
}

export function isGranularity(v: string | null | undefined): v is Granularity {
  return v === 'overview' || v === 'runtime' || v === 'detailed'
}
