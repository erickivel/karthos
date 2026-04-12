import { useQuery } from '@tanstack/react-query'
import { getSnapshot } from '@/api/graph'
import { useAppStore } from '@/stores/useAppStore'
import { useEffect } from 'react'
import type { GraphSnapshot } from '@/types/graph'

/** Ensure every node has top-level name/namespace (fall back to labels). */
function normalizeSnapshot(snapshot: GraphSnapshot): GraphSnapshot {
  return {
    ...snapshot,
    graph: {
      ...snapshot.graph,
      nodes: snapshot.graph.nodes.map((n) => ({
        ...n,
        labels: n.labels ?? {},
        name: n.name || n.labels?.['name'] || n.id,
        namespace: n.namespace || n.labels?.['namespace'] || '',
        cluster: n.cluster || '',
        status: n.status || 'unknown',
      })),
    },
  }
}

export function useGraphSnapshot() {
  const setConnectionStatus = useAppStore((s) => s.setConnectionStatus)

  const query = useQuery({
    queryKey: ['graph-snapshot'],
    queryFn: getSnapshot,
    refetchInterval: 30_000,
    select: normalizeSnapshot,
  })

  useEffect(() => {
    if (query.isError) {
      setConnectionStatus('disconnected')
    }
  }, [query.isError, setConnectionStatus])

  return query
}
