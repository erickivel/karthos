import type { GraphEdge, GraphNode } from '@/types/graph'
import type { CanonicalMetric, MetricCategory, MetricScope } from '@/types/metrics'

export type ExplorerMode = 'entity' | 'edge'

export interface EntityOption {
  id: string
  label: string
}

export interface EdgeOption {
  sourceId: string
  targetId: string
  label: string
}

export function filterMetricsForMode(
  metrics: CanonicalMetric[] | undefined,
  category: MetricCategory,
  mode: ExplorerMode,
): CanonicalMetric[] {
  const requiredScope: MetricScope = mode === 'entity' ? 'entity' : 'edge'
  return (metrics ?? []).filter(
    (metric) => metric.category === category && metric.scopes.includes(requiredScope),
  )
}

export function buildEntityOptions(nodes: GraphNode[] | undefined): EntityOption[] {
  return [...(nodes ?? [])].sort(compareNodes).map((node) => ({
    id: node.id,
    label: `[${node.kind}] ${node.namespace ? `${node.namespace}/` : ''}${node.name}`,
  }))
}

export function buildEdgeOptions(
  edges: GraphEdge[] | undefined,
  nodes: GraphNode[] | undefined,
): EdgeOption[] {
  const nodeByID = new Map((nodes ?? []).map((node) => [node.id, node]))
  const seen = new Set<string>()

  return (edges ?? [])
    .filter((edge) => edge.relation === 'calls')
    .filter((edge) => {
      const key = `${edge.source}|${edge.target}`
      if (seen.has(key)) return false
      seen.add(key)
      return true
    })
    .map((edge) => {
      const source = nodeByID.get(edge.source)
      const target = nodeByID.get(edge.target)
      return {
        sourceId: edge.source,
        targetId: edge.target,
        label: `${source?.name ?? edge.source} -> ${target?.name ?? edge.target}`,
      }
    })
}

function compareNodes(a: GraphNode, b: GraphNode): number {
  return (
    a.kind.localeCompare(b.kind) ||
    a.namespace.localeCompare(b.namespace) ||
    a.name.localeCompare(b.name) ||
    a.id.localeCompare(b.id)
  )
}
