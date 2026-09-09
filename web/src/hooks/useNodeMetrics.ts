import { useMemo } from 'react'
import { useQueries } from '@tanstack/react-query'
import { getAllSeries } from '@/api/metrics'
import type { CanonicalMetric, MetricSeries } from '@/types/metrics'
import { collectAllMetricNames } from '@/views/TopologyGraph/nodeMetrics'
import { usePollingEnabled } from './usePollingEnabled'

export interface NodeMetricValue {
  value?: number
  unit: string
  seriesCount: number
}

export type NodeMetricValues = Record<string, NodeMetricValue>

function latestSample(series: MetricSeries) {
  return [...series.samples].sort(
    (a, b) => new Date(b.timestamp).getTime() - new Date(a.timestamp).getTime(),
  )[0]
}

/** Queries only catalog-declared presentation metrics. */
export function useNodeMetrics(catalog: CanonicalMetric[] | undefined): Map<string, NodeMetricValues> {
  const pollingEnabled = usePollingEnabled()
  const metricNames = useMemo(() => {
    const declared = new Set((catalog ?? []).map((metric) => metric.name))
    return collectAllMetricNames().filter((name) => declared.has(name))
  }, [catalog])

  const queries = useQueries({
    queries: metricNames.map((metric) => ({
      queryKey: ['topology-metrics', metric],
      queryFn: () => getAllSeries({ metric_name: metric, page_size: 1000 }),
      refetchInterval: pollingEnabled ? 30_000 : false,
      staleTime: 15_000,
    })),
  })

  return useMemo(() => {
    const values = new Map<string, NodeMetricValues>()
    metricNames.forEach((metricName, index) => {
      const series = queries[index]?.data?.series ?? []
      const byEntity = new Map<string, MetricSeries[]>()
      for (const item of series) {
        const entityID = item.samples[0]?.entity_id
        if (!entityID) continue
        const items = byEntity.get(entityID) ?? []
        items.push(item)
        byEntity.set(entityID, items)
      }
      for (const [entityID, entitySeries] of byEntity) {
        const current = entitySeries.length === 1 ? latestSample(entitySeries[0]!) : undefined
        const entry = values.get(entityID) ?? {}
        entry[metricName] = {
          value: current?.value,
          unit: entitySeries[0]?.unit ?? '',
          seriesCount: entitySeries.length,
        }
        values.set(entityID, entry)
      }
    })
    return values
  }, [metricNames, queries])
}
