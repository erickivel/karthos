import { useState, useMemo } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQueries } from '@tanstack/react-query'
import { useMetricCatalog } from '@/hooks/useMetricCatalog'
import { getSeries } from '@/api/metrics'
import { EntitySelector } from './EntitySelector'
import { TimeRangeSelector, getTimeRange, type TimeRange } from './TimeRangeSelector'
import { CategoryTabs } from './CategoryTabs'
import { MetricCard } from './MetricCard'
import { RawSamplesTable } from './RawSamplesTable'
import type { MetricCategory, MetricSeriesResponse } from '@/types/metrics'
import styles from './MetricsExplorer.module.css'

export function MetricsExplorer() {
  const [searchParams, setSearchParams] = useSearchParams()
  const entityId = searchParams.get('entity') ?? ''
  const [timeRange, setTimeRange] = useState<TimeRange>('15m')
  const [category, setCategory] = useState<MetricCategory>('resource')

  const { data: catalog } = useMetricCatalog()

  const categoryMetrics = useMemo(
    () => catalog?.metrics.filter((m) => m.category === category) ?? [],
    [catalog, category],
  )

  // Memoize time range so from/to are stable between renders
  // (getTimeRange uses new Date() which changes every millisecond)
  const { from, to } = useMemo(() => getTimeRange(timeRange), [timeRange])

  // Fetch series for ALL metrics in the category in parallel
  const seriesQueries = useQueries({
    queries: categoryMetrics.map((metric) => ({
      queryKey: ['metrics', metric.name, entityId, from, to],
      queryFn: () => getSeries({ metric_name: metric.name, entity_id: [entityId], from, to }),
      enabled: !!entityId,
      refetchInterval: 30_000,
    })),
  })

  // Build a lookup from metric name to its series data
  const seriesByMetric = useMemo(() => {
    const map = new Map<string, MetricSeriesResponse>()
    categoryMetrics.forEach((metric, i) => {
      const result = seriesQueries[i]
      if (result?.data) {
        map.set(metric.name, result.data)
      }
    })
    return map
  }, [categoryMetrics, seriesQueries])

  // Collect all samples for the raw table
  const allSamples = useMemo(() => {
    const samples: Array<{ metric_name: string; entity_id: string; value: number; timestamp: string; unit: string }> = []
    for (const [, data] of seriesByMetric) {
      for (const s of data.series) {
        for (const sample of s.samples) {
          samples.push(sample)
        }
      }
    }
    return samples
  }, [seriesByMetric])

  const setEntityId = (id: string) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (id) {
        next.set('entity', id)
      } else {
        next.delete('entity')
      }
      return next
    })
  }

  return (
    <div className={styles.container}>
      <header className={styles.header}>
        <h1 className={styles.title}>Metrics Explorer</h1>
        <div className={styles.controls}>
          <EntitySelector value={entityId} onChange={setEntityId} />
          <TimeRangeSelector value={timeRange} onChange={setTimeRange} />
        </div>
      </header>

      <CategoryTabs active={category} onChange={setCategory} />

      {!entityId && (
        <div className={styles.prompt}>Select an entity to view metrics</div>
      )}

      {entityId && categoryMetrics.length > 0 && (
        <div className={styles.chartGrid}>
          {categoryMetrics.map((metric) => {
            const data = seriesByMetric.get(metric.name)
            const series = data?.series.find((s) => s.metric_name === metric.name)
            return (
              <MetricCard
                key={metric.name}
                name={metric.name}
                unit={metric.unit}
                samples={series?.samples ?? []}
              />
            )
          })}
        </div>
      )}

      {entityId && categoryMetrics.length === 0 && (
        <div className={styles.prompt}>No metrics defined for this category</div>
      )}

      {allSamples.length > 0 && (
        <RawSamplesTable samples={allSamples} />
      )}
    </div>
  )
}
