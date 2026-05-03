import { useEffect, useMemo, useState } from 'react'
import type { Node, Edge } from '@xyflow/react'
import type { GraphNode, GraphEdge, EntityKind, RelationType } from '@/types/graph'
import type { NodeMetricValues } from '@/hooks/useNodeMetrics'
import { getEdgeMetricKeyFromNodes, type EdgeMetricSnapshot } from './edgeMetrics'
import type { ProjectionMode } from './projection'
import type { LayoutResult, LayoutResultLike, MeasuredSize } from './layouts/types'
import { type LayoutAlgorithm, resolveStrategy } from './layouts/registry'

export interface LayoutInput {
  nodes: GraphNode[]
  edges: GraphEdge[]
  groupOf: Map<string, string>
  parentKind: EntityKind | null
  nodeSizes?: Map<string, MeasuredSize>
}

const EMPTY_RESULT: LayoutResult = { nodes: [], edges: [] }

function isPromise<T>(value: T | Promise<T>): value is Promise<T> {
  return typeof (value as { then?: unknown })?.then === 'function'
}

function runStrategy(
  input: LayoutInput,
  mode: ProjectionMode,
  algorithm: LayoutAlgorithm | undefined,
): LayoutResultLike {
  const { strategy } = resolveStrategy(mode, algorithm)
  return strategy.layout(input, { mode })
}

export function useGraphLayout(
  input: LayoutInput,
  mode: ProjectionMode,
  algorithm?: LayoutAlgorithm,
  nodeMetrics?: Map<string, NodeMetricValues>,
  edgeMetrics?: Map<string, EdgeMetricSnapshot>,
): { nodes: Node[]; edges: Edge[] } {
  // Layout depends only on graph structure, mode, algorithm, and the (optional)
  // measured node sizes — NOT on metric maps. Otherwise every metric poll would
  // re-run the layout (and cancel any in-flight async layout mid-resolve).
  const computed = useMemo<LayoutResultLike | { error: unknown }>(() => {
    try {
      return runStrategy(input, mode, algorithm)
    } catch (err) {
      console.error('[topology] layout failed', err)
      return { error: err }
    }
  }, [input, mode, algorithm])

  const initial = useMemo<LayoutResult>(() => {
    if (computed && typeof computed === 'object' && 'error' in computed) return EMPTY_RESULT
    return isPromise(computed) ? EMPTY_RESULT : computed
  }, [computed])

  const [resolved, setResolved] = useState<LayoutResult>(initial)

  useEffect(() => {
    if (computed && typeof computed === 'object' && 'error' in computed) {
      setResolved(EMPTY_RESULT)
      return
    }
    if (!isPromise(computed)) {
      setResolved(computed)
      return
    }
    let cancelled = false
    computed
      .then((result) => {
        if (!cancelled) setResolved(result)
      })
      .catch((err: unknown) => {
        if (cancelled) return
        console.error('[topology] async layout failed', err)
        setResolved(EMPTY_RESULT)
      })
    return () => {
      cancelled = true
    }
  }, [computed])

  return useMemo(
    () => decorateWithMetrics(resolved, nodeMetrics, edgeMetrics),
    [resolved, nodeMetrics, edgeMetrics],
  )
}

function decorateWithMetrics(
  layout: LayoutResult,
  nodeMetrics: Map<string, NodeMetricValues> | undefined,
  edgeMetrics: Map<string, EdgeMetricSnapshot> | undefined,
): LayoutResult {
  if (!nodeMetrics && !edgeMetrics) return layout
  return {
    nodes: layout.nodes.map((node) => ({
      ...node,
      data: {
        ...node.data,
        metrics: nodeMetrics?.get(node.id) ?? {},
      },
    })),
    edges: layout.edges.map((edge) => {
      const data = edge.data as
        | { relation?: RelationType; sourceName?: string; targetName?: string }
        | undefined
      const key =
        data?.relation && data.sourceName && data.targetName
          ? getEdgeMetricKeyFromNodes(data.relation, data.sourceName, data.targetName)
          : null
      return {
        ...edge,
        data: {
          ...edge.data,
          metrics: key ? edgeMetrics?.get(key) : undefined,
        },
      }
    }),
  }
}
