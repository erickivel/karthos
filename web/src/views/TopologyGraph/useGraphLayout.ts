import { useEffect, useMemo, useState } from 'react'
import type { Node, Edge } from '@xyflow/react'
import type { NodeMetricValues } from '@/hooks/useNodeMetrics'
import type { ProjectionMode } from './projection'
import type { LayoutInput, LayoutResult, LayoutResultLike } from './layouts/types'
import { resolveStrategy } from './layouts/registry'

const EMPTY_RESULT: LayoutResult = { nodes: [], edges: [] }

function isPromise<T>(value: T | Promise<T>): value is Promise<T> {
  return typeof (value as { then?: unknown })?.then === 'function'
}

function runStrategy(
  input: LayoutInput,
  mode: ProjectionMode,
): LayoutResultLike {
  return resolveStrategy(mode).layout(input, { mode })
}

export function useGraphLayout(
  input: LayoutInput,
  mode: ProjectionMode,
  nodeMetrics?: Map<string, NodeMetricValues>,
): { nodes: Node[]; edges: Edge[] } {
  // Layout depends on graph structure, mode, and optional measured
  // node sizes — NOT on metric maps.
  // Otherwise every metric poll would re-run the layout (and cancel any
  // in-flight async layout mid-resolve).
  const computed = useMemo<LayoutResultLike | { error: unknown }>(() => {
    try {
      return runStrategy(input, mode)
    } catch (err) {
      console.error('[topology] layout failed', err)
      return { error: err }
    }
  }, [input, mode])

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
    () => decorateWithMetrics(resolved, nodeMetrics),
    [resolved, nodeMetrics],
  )
}

function decorateWithMetrics(
  layout: LayoutResult,
  nodeMetrics: Map<string, NodeMetricValues> | undefined,
): LayoutResult {
  if (!nodeMetrics) return layout
  return {
    nodes: layout.nodes.map((node) => ({
      ...node,
      data: {
        ...node.data,
        metrics: nodeMetrics?.get(node.id) ?? {},
      },
    })),
    edges: layout.edges,
  }
}
