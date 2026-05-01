import { useMemo } from 'react'
import type { Node, Edge } from '@xyflow/react'
import type { GraphNode, GraphEdge, EntityKind } from '@/types/graph'
import type { NodeMetricValues } from '@/hooks/useNodeMetrics'
import type { EdgeMetricSnapshot } from './edgeMetrics'
import type { ProjectionMode } from './projection'
import type { LayoutStrategy } from './layouts/types'
import { ownershipLayout } from './layouts/ownershipLayout'
import { infrastructureLayout } from './layouts/infrastructureLayout'
import { trafficLayout } from './layouts/trafficLayout'

const STRATEGIES: Record<ProjectionMode, LayoutStrategy> = {
  ownership: ownershipLayout,
  infrastructure: infrastructureLayout,
  traffic: trafficLayout,
}

export interface LayoutInput {
  nodes: GraphNode[]
  edges: GraphEdge[]
  groupOf: Map<string, string>
  parentKind: EntityKind | null
}

export function useGraphLayout(
  input: LayoutInput,
  mode: ProjectionMode,
  nodeMetrics?: Map<string, NodeMetricValues>,
  edgeMetrics?: Map<string, EdgeMetricSnapshot>,
): { nodes: Node[]; edges: Edge[] } {
  return useMemo(() => {
    const strategy = STRATEGIES[mode]
    try {
      return strategy.layout(input, { mode, nodeMetrics, edgeMetrics })
    } catch (err) {
      // Layout engines can throw on malformed input. Render empty rather than crashing the view.
      console.error('[topology] layout failed', err)
      return { nodes: [], edges: [] }
    }
  }, [input, mode, nodeMetrics, edgeMetrics])
}
