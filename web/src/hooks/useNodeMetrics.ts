import { useMemo } from 'react'
import { useQueries } from '@tanstack/react-query'
import { getSeries } from '@/api/metrics'
import { collectAllMetricNames } from '@/views/TopologyGraph/nodeMetrics'
import { usePollingEnabled } from './usePollingEnabled'

// Per-entity metric snapshot keyed by metric name.
export type NodeMetricValues = Record<string, number>

/**
 * Fetches the latest value of every metric named by NODE_METRICS for every
 * entity that has a sample. Returns a Map<entityId, Record<metric, value>>
 * so EntityNode can look up values by (kind → spec → metric name).
 *
 * The query list is derived from NODE_METRICS — adding a new metric there
 * automatically extends the fetch.
 */
export function useNodeMetrics(): Map<string, NodeMetricValues> {
  const pollingEnabled = usePollingEnabled()
  const metricNames = useMemo(() => collectAllMetricNames(), [])

  const queries = useQueries({
    queries: metricNames.map((metric) => ({
      queryKey: ['node-metrics', metric],
      queryFn: () => getSeries({ metric_name: metric }),
      refetchInterval: pollingEnabled ? 30_000 : false,
      staleTime: 15_000,
    })),
  })

  return useMemo(() => {
    const map = new Map<string, NodeMetricValues>()
    for (let i = 0; i < metricNames.length; i++) {
      const metric = metricNames[i]
      const result = queries[i]
      if (!metric || !result?.data) continue
      for (const series of result.data.series) {
        for (const sample of series.samples) {
          let entry = map.get(sample.entity_id)
          if (!entry) {
            entry = {}
            map.set(sample.entity_id, entry)
          }
          if (entry[metric] === undefined) {
            entry[metric] = sample.value
          }
        }
      }
    }
    return map
  }, [queries, metricNames])
}
