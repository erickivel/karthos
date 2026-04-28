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
    relations: new Set<RelationType>(['contains', 'mounts', 'uses']),
    // Anything with a namespace, or namespaces themselves, intrinsically
    // belongs to the ownership layer.
    intrinsic: (e) => e.kind === 'namespace' || Boolean(e.namespace),
  },
  infrastructure: {
    id: 'infrastructure',
    kinds: new Set<EntityKind>(['node', 'pod', 'container', 'pvc']),
    relations: new Set<RelationType>(['runs_on', 'mounts']),
    intrinsic: (e) => e.kind === 'node',
  },
  traffic: {
    id: 'traffic',
    kinds: new Set<EntityKind>(['service', 'workload', 'ingress', 'pod']),
    relations: new Set<RelationType>(['calls', 'routes_to', 'exposes']),
  },
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
  const base = new Set(VISIBLE_KINDS_BY_GRANULARITY[granularity])
  if (mode === 'infrastructure' && granularity === 'detailed') {
    // Host-node grouping gets unreadable when every container is flattened
    // beside its pod. Keep infrastructure focused on schedulable runtime
    // objects; container detail remains available through ownership L3.
    base.delete('container')
  }
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

export function project(
  nodes: GraphNode[],
  edges: GraphEdge[],
  mode: ProjectionMode,
  granularity: Granularity,
  options: { includeOrphans?: boolean } = {},
): ProjectionResult {
  const visibleKinds = visibleKindsFor(mode, granularity)
  const parentKind = parentKindFor(mode)
  const spec = LAYER_SPECS[mode]
  const includeOrphans = options.includeOrphans ?? false

  const kindFiltered = nodes.filter((n) => visibleKinds.has(n.kind))

  let visibleNodes: GraphNode[]
  if (includeOrphans) {
    visibleNodes = kindFiltered
  } else {
    const participants = computeParticipants(kindFiltered, edges, spec)
    visibleNodes = kindFiltered.filter((n) => participants.has(n.id))
  }

  visibleNodes = sortEntities(visibleNodes)

  const visibleIds = new Set(visibleNodes.map((n) => n.id))
  const visibleEdgesUnsorted = edges.filter(
    (e) => visibleIds.has(e.source) && visibleIds.has(e.target),
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
    const nodeEntitiesById = new Map<string, GraphNode>()
    for (const n of visibleNodes) if (n.kind === 'node') nodeEntitiesById.set(n.id, n)

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
      if (n.kind === 'node') continue
      if (n.kind === 'pod') {
        const host = podToHostNode.get(n.id)
        if (host) groupOf.set(n.id, host)
      } else if (n.kind === 'container') {
        const podId = containerToPod.get(n.id)
        if (podId) {
          const host = podToHostNode.get(podId)
          if (host) groupOf.set(n.id, host)
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
