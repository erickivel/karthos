import { useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQueries } from '@tanstack/react-query'
import { useMetricCatalog } from '@/hooks/useMetricCatalog'
import { useGraphSnapshot } from '@/hooks/useGraphSnapshot'
import { useEntities } from '@/hooks/useEntities'
import { getSeries } from '@/api/metrics'
import { EntitySelector } from './EntitySelector'
import { TimeRangeSelector, getTimeRange, type TimeRange } from './TimeRangeSelector'
import { CategoryTabs } from './CategoryTabs'
import { MetricCard } from './MetricCard'
import { RawSamplesTable } from './RawSamplesTable'
import type { GraphEdge, GraphNode } from '@/types/graph'
import type { MetricCategory, MetricSeries, MetricSample } from '@/types/metrics'
import styles from './MetricsExplorer.module.css'

type ExplorerMode = 'entity' | 'edge'

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
  const { data: entityData } = useEntities({ page_size: 1000 })

  const categoryMetrics = useMemo(
    () => catalog?.metrics.filter((m) => m.category === category) ?? [],
    [catalog, category],
  )

  const edgeOptions = useMemo(() => {
    if (!snapshot) return []

    const nodeByID = new Map<string, GraphNode>(snapshot.graph.nodes.map((node) => [node.id, node]))
    const entityByID = new Map((entityData?.entities ?? []).map((entity) => [entity.id, entity]))
    const seen = new Set<string>()

    return snapshot.graph.edges
      .filter((edge) => edge.relation === 'calls')
      .filter((edge) => {
        const key = `${edge.source}|${edge.target}`
        if (seen.has(key)) return false
        seen.add(key)
        return true
      })
      .map((edge) => toEdgeOption(edge, nodeByID, entityByID))
  }, [entityData?.entities, snapshot])

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
            <EntitySelector value={entityId} onChange={setEntityId} />
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

function toEdgeOption(
  edge: GraphEdge,
  nodeByID: Map<string, GraphNode>,
  entityByID: Map<string, { name: string; namespace: string }>,
) {
  const source = nodeByID.get(edge.source) ?? entityByID.get(edge.source)
  const target = nodeByID.get(edge.target) ?? entityByID.get(edge.target)

  return {
    sourceId: edge.source,
    targetId: edge.target,
    label: `${source?.name ?? edge.source} -> ${target?.name ?? edge.target}`,
  }
}
