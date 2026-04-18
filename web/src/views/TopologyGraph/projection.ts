import type { EntityKind, GraphEdge, GraphNode } from '@/types/graph'

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
  if (mode === 'traffic') {
    base.delete('namespace')
    base.delete('node')
  } else if (mode === 'ownership') {
    base.delete('node')
  } else if (mode === 'infrastructure') {
    base.delete('namespace')
  }
  return base
}

export function parentKindFor(mode: ProjectionMode): EntityKind | null {
  if (mode === 'ownership') return 'namespace'
  if (mode === 'infrastructure') return 'node'
  return null
}

export function project(
  nodes: GraphNode[],
  edges: GraphEdge[],
  mode: ProjectionMode,
  granularity: Granularity,
): ProjectionResult {
  const visibleKinds = visibleKindsFor(mode, granularity)
  const parentKind = parentKindFor(mode)

  const visibleNodes = nodes.filter((n) => visibleKinds.has(n.kind))
  const visibleIds = new Set(visibleNodes.map((n) => n.id))
  const visibleEdges = edges.filter(
    (e) => visibleIds.has(e.source) && visibleIds.has(e.target),
  )

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
