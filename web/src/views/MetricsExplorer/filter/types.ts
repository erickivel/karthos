import type { EntityKind, GraphNode } from '@/types/graph'

/**
 * Single-entity filter state. URL holds only `entityId` (the shareable bit);
 * the rest is component-local scaffolding.
 */
export interface FilterState {
  namespace: string
  kind: EntityKind | ''
  entityId: string
  query: string
  status: string
}

export const EMPTY_FILTER: FilterState = {
  namespace: '',
  kind: '',
  entityId: '',
  query: '',
  status: '',
}

export interface FilterOption {
  label: string
  value: string
  meta?: string
  color?: string
  health?: NodeHealth
}

export type NodeHealth = 'healthy' | 'degraded' | 'down'

/**
 * Coarse health bucketing derived from `GraphNode.status`. The graph store
 * surfaces free-form status strings; we normalise to three buckets so the
 * UI can render a deterministic dot.
 */
export function nodeHealth(status: string | undefined): NodeHealth {
  const s = (status ?? '').toLowerCase()
  if (s === 'ready' || s === 'running' || s === 'healthy' || s === 'active' || s === 'available') {
    return 'healthy'
  }
  if (s === 'failed' || s === 'down' || s === 'unhealthy' || s === 'crashloopbackoff' || s === 'error') {
    return 'down'
  }
  return 'degraded'
}

/**
 * Applies the filter state to a node list. Pure — used by both the entity
 * list and the global search.
 */
export function applyFilters(nodes: GraphNode[] | undefined, state: FilterState): GraphNode[] {
  return (nodes ?? []).filter((node) => {
    if (state.namespace && node.namespace !== state.namespace) return false
    if (state.kind && node.kind !== state.kind) return false
    if (state.status && nodeHealth(node.status) !== state.status) return false
    if (state.query) {
      const q = state.query.toLowerCase()
      const hay = `${node.name} ${node.namespace} ${node.kind} ${node.status ?? ''}`.toLowerCase()
      if (!hay.includes(q)) return false
    }
    return true
  })
}

export function compareNodes(a: GraphNode, b: GraphNode): number {
  return (
    a.kind.localeCompare(b.kind) ||
    a.namespace.localeCompare(b.namespace) ||
    a.name.localeCompare(b.name) ||
    a.id.localeCompare(b.id)
  )
}

export function nodeLabel(node: GraphNode): string {
  return `${node.namespace ? `${node.namespace}/` : ''}${node.name}`
}
