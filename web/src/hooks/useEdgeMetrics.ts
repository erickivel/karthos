import { useMemo } from 'react'
import { useQueries } from '@tanstack/react-query'
import { getSamples } from '@/api/metrics'
import type { MetricSample } from '@/types/metrics'
import { usePollingEnabled } from './usePollingEnabled'
import {
  getEdgeMetricKeyFromLabels,
  type EdgeMetricSnapshot,
} from '@/views/TopologyGraph/edgeMetrics'

const EDGE_METRIC_NAMES = [
  'servicegraph.request.count',
  'servicegraph.request.duration',
  'servicegraph.request.error.count',
  'servicegraph.request.error.ratio',
] as const

function applySample(
  map: Map<string, EdgeMetricSnapshot>,
  sample: MetricSample,
) {
  const key = getEdgeMetricKeyFromLabels(sample.labels)
  if (!key) return

  const existing = map.get(key) ?? {}

  switch (sample.metric_name) {
    case 'servicegraph.request.count':
      if (existing.requestRate === undefined) existing.requestRate = sample.value
      break
    case 'servicegraph.request.duration':
      if (existing.latencyNs === undefined) existing.latencyNs = sample.value
      break
    case 'servicegraph.request.error.count':
      if (existing.errorRate === undefined) existing.errorRate = sample.value
      break
    case 'servicegraph.request.error.ratio':
      if (existing.errorRatio === undefined) existing.errorRatio = sample.value
      break
    default:
      return
  }

  const labels = sample.labels
  if (labels) {
    if (existing.source === undefined && labels.source) {
      existing.source = labels.source
    }
    if (existing.mtls === undefined && labels['mesh.security.policy']) {
      existing.mtls = labels['mesh.security.policy'] === 'mutual_tls'
    }
    if (existing.protocol === undefined && labels['network.protocol.name']) {
      existing.protocol = labels['network.protocol.name']
    }
    if (existing.peerSource === undefined && labels['mesh.peer.source.principal']) {
      existing.peerSource = labels['mesh.peer.source.principal']
    }
    if (existing.peerDestination === undefined && labels['mesh.peer.destination.principal']) {
      existing.peerDestination = labels['mesh.peer.destination.principal']
    }
  }

  map.set(key, existing)
}

export function useEdgeMetrics(): Map<string, EdgeMetricSnapshot> {
  const pollingEnabled = usePollingEnabled()
  const queries = useQueries({
    queries: EDGE_METRIC_NAMES.map((metricName) => ({
      queryKey: ['edge-metrics', metricName],
      queryFn: () => getSamples({ metric_name: metricName, page_size: 1000 }),
      refetchInterval: pollingEnabled ? 30_000 : false,
      staleTime: 15_000,
    })),
  })

  return useMemo(() => {
    const map = new Map<string, EdgeMetricSnapshot>()

    for (const query of queries) {
      if (!query.data) continue

      for (const sample of query.data.samples) {
        applySample(map, sample)
      }
    }

    return map
  }, [queries])
}
