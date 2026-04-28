import { useEffect, useState } from 'react'
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
  const [result, setResult] = useState<{ nodes: Node[]; edges: Edge[] }>({
    nodes: [],
    edges: [],
  })

  useEffect(() => {
    let cancelled = false
    const strategy = STRATEGIES[mode]
    strategy
      .layout(input, { mode, nodeMetrics, edgeMetrics })
      .then((laid) => {
        if (!cancelled) setResult(laid)
      })
      .catch((err) => {
        // ELK can throw on malformed input. Render empty rather than crashing the view.
        if (!cancelled) {
          console.error('[topology] layout failed', err)
          setResult({ nodes: [], edges: [] })
        }
      })
    return () => {
      cancelled = true
    }
  }, [input, mode, nodeMetrics, edgeMetrics])

  return result
}
