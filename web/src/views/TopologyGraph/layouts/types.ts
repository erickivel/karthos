import type { Node, Edge } from '@xyflow/react'
import type { GraphNode, GraphEdge, EntityKind } from '@/types/graph'
import type { ProjectionMode } from '../projection'

export interface MeasuredSize {
  width: number
  height: number
}

export interface LayoutInput {
  nodes: GraphNode[]
  edges: GraphEdge[]
  groupOf: Map<string, string>
  parentKind: EntityKind | null
  // Populated after React Flow has measured the rendered DOM. Strategies that
  // need precise box dimensions (ELK) read from here; sync strategies that work
  // off the constants below ignore it.
  nodeSizes?: Map<string, MeasuredSize>
}

export interface LayoutDeps {
  mode: ProjectionMode
}

export interface LayoutResult {
  nodes: Node[]
  edges: Edge[]
}

export type LayoutResultLike = LayoutResult | Promise<LayoutResult>

export interface LayoutStrategy {
  layout(input: LayoutInput, deps: LayoutDeps): LayoutResultLike
}

// The EntityNode card CSS is `width: 250px` (see EntityNode.module.css). Keep
// this in sync — every layout uses it as the authoritative width unless real
// measurements are supplied via `LayoutInput.nodeSizes`.
export const NODE_WIDTH = 250
export const NODE_HEIGHT_DEFAULT = 170
export const NODE_HEIGHT_SMALL = 90
export const SMALL_KINDS: EntityKind[] = ['configmap', 'secret', 'pvc']
