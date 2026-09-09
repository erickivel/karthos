import type { EntityKind, GraphNode } from '@/types/graph'
import { entityKindSearchText } from '@/utils/entityDisplay'

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

/**
 * Public health-status union. The backend translates the integer-encoded
 * `karthos.{pod,container}.health.status` enum to one of these strings at the
 * API boundary (see `pkg/core/metrics/health_status.go`); the frontend just
 * renders. No mapping logic lives here.
 */
export type NodeHealth =
  | 'healthy'
  | 'running_not_ready'
  | 'restarting'
  | 'pending'
  | 'succeeded'
  | 'failed'
  | 'unknown'

const KNOWN_HEALTHS: ReadonlySet<NodeHealth> = new Set([
  'healthy',
  'running_not_ready',
  'restarting',
  'pending',
  'succeeded',
  'failed',
  'unknown',
])

/**
 * Narrow the free-form `GraphNode.status` string to the public NodeHealth
 * union. Anything outside the set returned by the backend is reported as
 * `unknown` so consumers don't have to guard against it.
 */
export function nodeHealth(status: string | undefined): NodeHealth {
  const s = status ?? ''
  return KNOWN_HEALTHS.has(s as NodeHealth) ? (s as NodeHealth) : 'unknown'
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
      const hay = `${node.name} ${node.namespace} ${entityKindSearchText(node)} ${node.status ?? ''}`.toLowerCase()
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
