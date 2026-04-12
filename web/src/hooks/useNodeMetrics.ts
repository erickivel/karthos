import { useMemo } from 'react'
import { useQueries } from '@tanstack/react-query'
import { getSeries } from '@/api/metrics'

export interface NodeMetric {
  cpu?: number
  mem?: number
}

/** Fetches latest CPU and memory values for all entities, returns a map by entity_id. */
export function useNodeMetrics(): Map<string, NodeMetric> {
  const queries = useQueries({
    queries: ['cpu_usage_millicores', 'memory_usage_bytes'].map((metric) => ({
      queryKey: ['node-metrics', metric],
      queryFn: () => getSeries({ metric_name: metric }),
      refetchInterval: 30_000,
      staleTime: 15_000,
    })),
  })

  return useMemo(() => {
    const map = new Map<string, NodeMetric>()
    const [cpuResult, memResult] = queries

    // Process CPU samples — get latest per entity
    if (cpuResult?.data) {
      for (const series of cpuResult.data.series) {
        for (const sample of series.samples) {
          const existing = map.get(sample.entity_id)
          if (!existing) {
            map.set(sample.entity_id, { cpu: sample.value })
          } else if (existing.cpu === undefined) {
            existing.cpu = sample.value
          }
        }
      }
    }

    // Process memory samples — get latest per entity
    if (memResult?.data) {
      for (const series of memResult.data.series) {
        for (const sample of series.samples) {
          const existing = map.get(sample.entity_id)
          if (!existing) {
            map.set(sample.entity_id, { mem: sample.value })
          } else if (existing.mem === undefined) {
            existing.mem = sample.value
          }
        }
      }
    }

    return map
  }, [queries])
}
