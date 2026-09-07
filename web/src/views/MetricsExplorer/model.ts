import type { GraphEdge } from '@/types/graph'

/** Current-graph edge selection intentionally excludes retained historical telemetry. */
export function hasCurrentCallsEdge(
  edges: GraphEdge[] | undefined,
  sourceId: string,
  targetId: string,
): boolean {
  return Boolean(sourceId && targetId && edges?.some(
    (edge) => edge.relation === 'calls' && edge.source === sourceId && edge.target === targetId,
  ))
}
