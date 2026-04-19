import { useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQueries } from '@tanstack/react-query'
import { useMetricCatalog } from '@/hooks/useMetricCatalog'
import { useGraphSnapshot } from '@/hooks/useGraphSnapshot'
import { getSeries } from '@/api/metrics'
import { EntitySelector } from './EntitySelector'
import { TimeRangeSelector, getTimeRange, type TimeRange } from './TimeRangeSelector'
import { CategoryTabs } from './CategoryTabs'
import { MetricCard } from './MetricCard'
import { RawSamplesTable } from './RawSamplesTable'
import type { MetricCategory, MetricSeries, MetricSample } from '@/types/metrics'
import { buildEdgeOptions, buildEntityOptions, filterMetricsForMode, type ExplorerMode } from './model'
import styles from './MetricsExplorer.module.css'

const ENTITY_MODE_CATEGORIES: MetricCategory[] = ['resource', 'traffic', 'latency', 'error', 'capacity', 'health']
const EDGE_MODE_CATEGORIES: MetricCategory[] = ['traffic', 'latency', 'error']

export function MetricsExplorer() {
  const [searchParams, setSearchParams] = useSearchParams()
  const mode = (searchParams.get('mode') as ExplorerMode | null) ?? 'entity'
  const entityId = searchParams.get('entity') ?? ''
  const edgeSourceId = searchParams.get('edgeSource') ?? ''
  const edgeTargetId = searchParams.get('edgeTarget') ?? ''
  const [timeRange, setTimeRange] = useState<TimeRange>('15m')
  const [category, setCategory] = useState<MetricCategory>('resource')

  const { data: catalog } = useMetricCatalog()
  const { data: snapshot } = useGraphSnapshot()

  const categoryMetrics = useMemo(
    () => filterMetricsForMode(catalog?.metrics, category, mode),
    [catalog?.metrics, category, mode],
  )

  const entityOptions = useMemo(() => buildEntityOptions(snapshot?.graph.nodes), [snapshot?.graph.nodes])

  const edgeOptions = useMemo(
    () => buildEdgeOptions(snapshot?.graph.edges, snapshot?.graph.nodes),
    [snapshot?.graph.edges, snapshot?.graph.nodes],
  )

  // Memoize time range so from/to are stable between renders
  // (getTimeRange uses new Date() which changes every millisecond)
  const { from, to } = useMemo(() => getTimeRange(timeRange), [timeRange])
  const isEntityMode = mode === 'entity'
  const visibleCategories = isEntityMode ? ENTITY_MODE_CATEGORIES : EDGE_MODE_CATEGORIES
  const hasSelection = isEntityMode ? !!entityId : !!edgeSourceId && !!edgeTargetId

  useEffect(() => {
    if (!visibleCategories.includes(category)) {
      setCategory(visibleCategories[0] ?? 'traffic')
    }
  }, [category, visibleCategories])

  // Fetch series for ALL metrics in the category in parallel
  const seriesQueries = useQueries({
    queries: categoryMetrics.map((metric) => ({
      queryKey: ['metrics', mode, metric.name, entityId, edgeSourceId, edgeTargetId, from, to],
      queryFn: () =>
        getSeries(
          isEntityMode
            ? { metric_name: metric.name, entity_id: [entityId], from, to }
            : { metric_name: metric.name, edge_source_id: edgeSourceId, edge_target_id: edgeTargetId, from, to },
        ),
      enabled: hasSelection,
      refetchInterval: 30_000,
    })),
  })

  // Build a lookup from metric name to its series data
  const seriesByMetric = useMemo(() => {
    const map = new Map<string, MetricSeries>()
    categoryMetrics.forEach((metric, i) => {
      const result = seriesQueries[i]
      if (result?.data?.series?.[0]) {
        map.set(metric.name, result.data.series[0])
      }
    })
    return map
  }, [categoryMetrics, seriesQueries])

  // Collect all samples for the raw table
  const allSamples = useMemo(() => {
    const samples: MetricSample[] = []
    for (const [, series] of seriesByMetric) {
      for (const sample of series.samples) {
        samples.push(sample)
      }
    }
    return samples
  }, [seriesByMetric])

  const selectedEdge = useMemo(
    () =>
      edgeOptions.find((option) => option.sourceId === edgeSourceId && option.targetId === edgeTargetId) ?? null,
    [edgeOptions, edgeSourceId, edgeTargetId],
  )

  const setEntityId = (id: string) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      next.set('mode', 'entity')
      if (id) {
        next.set('entity', id)
      } else {
        next.delete('entity')
      }
      next.delete('edgeSource')
      next.delete('edgeTarget')
      return next
    })
  }

  const setMode = (nextMode: ExplorerMode) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      next.set('mode', nextMode)
      if (nextMode === 'entity') {
        next.delete('edgeSource')
        next.delete('edgeTarget')
      } else {
        next.delete('entity')
      }
      return next
    })
  }

  const setEdge = (value: string) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      next.set('mode', 'edge')
      next.delete('entity')
      if (!value) {
        next.delete('edgeSource')
        next.delete('edgeTarget')
        return next
      }

      const [sourceId, targetId] = value.split('|')
      next.set('edgeSource', sourceId ?? '')
      next.set('edgeTarget', targetId ?? '')
      return next
    })
  }

  return (
    <div className={styles.container}>
      <header className={styles.header}>
        <h1 className={styles.title}>Metrics Explorer</h1>
        <div className={styles.controls}>
          <div className={styles.modeSwitch}>
            <button
              type="button"
              className={styles.modeButton}
              data-active={isEntityMode}
              onClick={() => setMode('entity')}
            >
              Entity
            </button>
            <button
              type="button"
              className={styles.modeButton}
              data-active={!isEntityMode}
              onClick={() => setMode('edge')}
            >
              Edge
            </button>
          </div>
          {isEntityMode ? (
            <EntitySelector value={entityId} onChange={setEntityId} options={entityOptions} />
          ) : (
            <select
              className={styles.select}
              value={edgeSourceId && edgeTargetId ? `${edgeSourceId}|${edgeTargetId}` : ''}
              onChange={(e) => setEdge(e.target.value)}
              aria-label="Select edge"
            >
              <option value="">Select edge...</option>
              {edgeOptions.map((edge) => (
                <option key={`${edge.sourceId}|${edge.targetId}`} value={`${edge.sourceId}|${edge.targetId}`}>
                  {edge.label}
                </option>
              ))}
            </select>
          )}
          <TimeRangeSelector value={timeRange} onChange={setTimeRange} />
        </div>
        {selectedEdge && (
          <div className={styles.selectionSummary}>
            Viewing edge metrics for {selectedEdge.label}
          </div>
        )}
      </header>

      <CategoryTabs active={category} categories={visibleCategories} onChange={setCategory} />

      {!hasSelection && (
        <div className={styles.prompt}>
          {isEntityMode ? 'Select an entity to view metrics' : 'Select a graph edge to view edge metrics'}
        </div>
      )}

      {hasSelection && categoryMetrics.length > 0 && (
        <div className={styles.chartGrid}>
          {categoryMetrics.map((metric) => {
            const series = seriesByMetric.get(metric.name)
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

      {hasSelection && categoryMetrics.length === 0 && (
        <div className={styles.prompt}>No metrics defined for this category</div>
      )}

      {allSamples.length > 0 && (
        <RawSamplesTable samples={allSamples} />
      )}
    </div>
  )
}
