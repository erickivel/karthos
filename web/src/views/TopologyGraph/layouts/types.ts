import type { Node, Edge } from '@xyflow/react'
import type { GraphNode, GraphEdge, EntityKind } from '@/types/graph'
import type { NodeMetricValues } from '@/hooks/useNodeMetrics'
import type { EdgeMetricSnapshot } from '../edgeMetrics'
import type { ProjectionMode } from '../projection'

export interface LayoutInput {
  nodes: GraphNode[]
  edges: GraphEdge[]
  groupOf: Map<string, string>
  parentKind: EntityKind | null
}

export interface LayoutDeps {
  mode: ProjectionMode
  nodeMetrics?: Map<string, NodeMetricValues>
  edgeMetrics?: Map<string, EdgeMetricSnapshot>
}

export interface LayoutResult {
  nodes: Node[]
  edges: Edge[]
}

export interface LayoutStrategy {
  layout(input: LayoutInput, deps: LayoutDeps): LayoutResult
}

export const NODE_WIDTH = 240
export const NODE_HEIGHT_DEFAULT = 170
export const NODE_HEIGHT_SMALL = 90
export const SMALL_KINDS: EntityKind[] = ['configmap', 'secret', 'pvc']
