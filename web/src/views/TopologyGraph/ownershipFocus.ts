import type { Edge } from '@xyflow/react'
import type { ProjectionMode } from './projection'
import type { LayoutResult } from './layouts/types'

export interface DependencyLinkCounts {
  dependencies: number
  dependents: number
}

const DEPENDENCY_RELATIONS = new Set(['uses', 'mounts'])

export function applyOwnershipDependencyFocus(
  layout: LayoutResult,
  mode: ProjectionMode,
): LayoutResult {
  if (mode !== 'ownership') return layout

  const counts = new Map<string, DependencyLinkCounts>()
  for (const edge of layout.edges) {
    const relation = relationOf(edge)
    if (!isDependencyRelation(relation)) continue
    increment(counts, edge.source, 'dependencies')
    increment(counts, edge.target, 'dependents')
  }

  return {
    nodes: layout.nodes.map((node) => ({
      ...node,
      data: {
        ...node.data,
        ...(counts.has(node.id) && { dependencyLinks: counts.get(node.id) }),
      },
    })),
    edges: layout.edges.filter((edge) => !isDependencyRelation(relationOf(edge))),
  }
}

function relationOf(edge: Edge): string | undefined {
  const data = edge.data as { relation?: unknown } | undefined
  return typeof data?.relation === 'string' ? data.relation : undefined
}

function isDependencyRelation(relation: string | undefined): boolean {
  return relation !== undefined && DEPENDENCY_RELATIONS.has(relation)
}

function increment(
  counts: Map<string, DependencyLinkCounts>,
  nodeID: string,
  direction: keyof DependencyLinkCounts,
): void {
  const current = counts.get(nodeID) ?? { dependencies: 0, dependents: 0 }
  current[direction] += 1
  counts.set(nodeID, current)
}
