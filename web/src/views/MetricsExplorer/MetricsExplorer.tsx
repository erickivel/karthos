import { useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQueries } from '@tanstack/react-query'
import { useMetricCatalog } from '@/hooks/useMetricCatalog'
import { useGraphSnapshot } from '@/hooks/useGraphSnapshot'
import { getSeries } from '@/api/metrics'
import { getTimeRange, type TimeRange } from './TimeRangeSelector'
import { CategoryTabs } from './CategoryTabs'
import { MetricCard } from './MetricCard'
import { RawSamplesTable } from './RawSamplesTable'
import { SelectionSummary } from './SelectionSummary'
import { GlobalEntitySearch } from './filter/GlobalEntitySearch'
import { FilterBreadcrumbBuilder } from './filter/FilterBreadcrumbBuilder'
import { EdgeSelector } from './edge/EdgeSelector'
import { EMPTY_FILTER, type FilterState } from './filter/types'
import { applicableMetrics, visibleCategoriesForSelection, type ExplorerMode } from './applicability/applicability'
import { MetricCatalogTable } from './catalog/MetricCatalogTable'
import type { GraphNode } from '@/types/graph'
import type { CanonicalMetric, MetricCategory, MetricSample, MetricSeries } from '@/types/metrics'
import styles from './MetricsExplorer.module.css'

const SYNCED_CURSOR_ID = 'metrics-explorer'

export function MetricsExplorer() {
  const [searchParams, setSearchParams] = useSearchParams()
  const rawMode = (searchParams.get('mode') as ExplorerMode | null) ?? 'entity'
  const mode: ExplorerMode = rawMode === 'edge' || rawMode === 'catalog' ? rawMode : 'entity'
  const entityId = searchParams.get('entity') ?? ''
  const edgeSourceId = searchParams.get('edgeSource') ?? ''
  const edgeTargetId = searchParams.get('edgeTarget') ?? ''
  const isEntityMode = mode === 'entity'
  const isEdgeMode = mode === 'edge'
  const isCatalogMode = mode === 'catalog'

  const [timeRange, setTimeRange] = useState<TimeRange>('15m')
  const [category, setCategory] = useState<MetricCategory>('resource')
  const [filter, setFilter] = useState<FilterState>(EMPTY_FILTER)

  const { data: catalog } = useMetricCatalog()
  const { data: snapshot } = useGraphSnapshot()
  const nodes = snapshot?.graph.nodes
  const edges = snapshot?.graph.edges

  const { from, to } = useMemo(() => getTimeRange(timeRange), [timeRange])

  const nodeById = useMemo(
    () => new Map((nodes ?? []).map((n) => [n.id, n])),
    [nodes],
  )

  // Sync URL → local filter state when the URL carries an entity id (deep-link
  // from NodeInspector) but the local filter doesn't yet reflect it.
  useEffect(() => {
    if (!isEntityMode || !entityId) return
    if (filter.entityId === entityId) return
    const node = nodeById.get(entityId)
    if (!node) return
    setFilter({
      namespace: node.namespace || '',
      kind: node.kind,
      entityId: node.id,
      query: '',
      status: '',
    })
  }, [isEntityMode, entityId, nodeById, filter.entityId])

  const selectedNode = isEntityMode && filter.entityId ? nodeById.get(filter.entityId) ?? null : null
  const selectedKind = selectedNode?.kind ?? null
  const applicabilityContext = useMemo(
    () => ({ labels: selectedNode?.labels }),
    [selectedNode?.labels],
  )

  const visibleCategories = useMemo(
    () => visibleCategoriesForSelection(catalog?.metrics, isEntityMode ? selectedKind : null, mode, applicabilityContext),
    [catalog?.metrics, isEntityMode, selectedKind, mode, applicabilityContext],
  )

  const categoryMetrics = useMemo(
    () =>
      applicableMetrics(
        catalog?.metrics,
        category,
        isEntityMode ? selectedKind : null,
        mode,
        applicabilityContext,
      ),
    [catalog?.metrics, category, mode, selectedKind, isEntityMode, applicabilityContext],
  )

  const hasSelection = isEntityMode ? Boolean(selectedNode) : isEdgeMode ? Boolean(edgeSourceId && edgeTargetId) : false

  // Snap to a still-visible category whenever the visibility set changes.
  useEffect(() => {
    if (visibleCategories.length === 0) return
    if (!visibleCategories.includes(category)) {
      setCategory(visibleCategories[0]!)
    }
  }, [category, visibleCategories])

  const seriesQueries = useQueries({
    queries: categoryMetrics.map((metric) => ({
      queryKey: [
        'metrics',
        mode,
        metric.name,
        isEntityMode ? filter.entityId : '',
        edgeSourceId,
        edgeTargetId,
        from,
        to,
      ],
      queryFn: () =>
        isEntityMode
          ? getSeries({ metric_name: metric.name, entity_id: [filter.entityId], from, to })
          : getSeries({
              metric_name: metric.name,
              edge_source_id: edgeSourceId,
              edge_target_id: edgeTargetId,
              from,
              to,
            }),
      enabled: !isCatalogMode && hasSelection,
      refetchInterval: 30_000,
    })),
  })

  const seriesByMetric = useMemo(() => {
    const map = new Map<string, MetricSeries>()
    categoryMetrics.forEach((metric, i) => {
      const series = seriesQueries[i]?.data?.series?.[0]
      if (series) map.set(metric.name, series)
    })
    return map
  }, [categoryMetrics, seriesQueries])

  const allSamples = useMemo(() => {
    const out: MetricSample[] = []
    for (const [, series] of seriesByMetric) {
      for (const sample of series.samples) out.push(sample)
    }
    return out
  }, [seriesByMetric])

  const setMode = (next: ExplorerMode) => {
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev)
      np.set('mode', next)
      if (next === 'entity') {
        np.delete('edgeSource')
        np.delete('edgeTarget')
        // Re-hydrate the entity URL param from local filter state so a
        // share-link copied after toggling Edge → Entity points at the
        // entity the user was viewing rather than to nothing.
        if (filter.entityId) np.set('entity', filter.entityId)
      } else if (next === 'edge') {
        np.delete('entity')
      } else {
        np.delete('entity')
        np.delete('edgeSource')
        np.delete('edgeTarget')
      }
      return np
    })
  }

  const handleOpenInExplorer = (metric: CanonicalMetric, targetMode: 'entity' | 'edge') => {
    // Catalog → samples view: switch the URL mode and pre-set the category
    // tab so the user sees the metric's family. Entity metrics with one
    // applicable kind also pre-scope the entity picker, while clearing any
    // stale entity/edge selection from the previous view.
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev)
      np.set('mode', targetMode)
      np.delete('entity')
      np.delete('edgeSource')
      np.delete('edgeTarget')
      return np
    })
    if (targetMode === 'entity') {
      const appliesTo = metric.applies_to ?? []
      setFilter({
        ...EMPTY_FILTER,
        kind: appliesTo.length === 1 ? appliesTo[0]! : '',
      })
    } else {
      setFilter(EMPTY_FILTER)
    }
    setCategory(metric.category)
  }

  const writeEntityToUrl = (id: string) => {
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev)
      np.set('mode', 'entity')
      np.delete('edgeSource')
      np.delete('edgeTarget')
      if (id) np.set('entity', id)
      else np.delete('entity')
      return np
    })
  }

  const handleFilterChange = (next: FilterState) => {
    setFilter(next)
    if (isEntityMode) writeEntityToUrl(next.entityId)
  }

  const handleGlobalSelect = (node: GraphNode) => {
    setFilter({
      namespace: node.namespace || '',
      kind: node.kind,
      entityId: node.id,
      query: '',
      status: '',
    })
    writeEntityToUrl(node.id)
  }

  const handleEdgeSelect = (sourceId: string, targetId: string) => {
    setSearchParams((prev) => {
      const np = new URLSearchParams(prev)
      np.set('mode', 'edge')
      np.delete('entity')
      np.set('edgeSource', sourceId)
      np.set('edgeTarget', targetId)
      return np
    })
  }

  return (
    <div className={styles.container}>
      <header className={styles.header}>
        <div className={styles.titleRow}>
          <span className={styles.eyebrow}>Observability · Metrics</span>
          <h1 className={styles.title}>Metrics Explorer</h1>
        </div>
        <div className={styles.modeSwitch} role="tablist" aria-label="Explorer mode">
          <button
            type="button"
            role="tab"
            aria-selected={isEntityMode}
            className={styles.modeButton}
            data-active={isEntityMode}
            onClick={() => setMode('entity')}
          >
            <EntityIcon />
            <span className={styles.modeLabel}>
              <span className={styles.modeName}>Entity</span>
              <span className={styles.modeHint}>node metrics</span>
            </span>
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={isEdgeMode}
            className={styles.modeButton}
            data-active={isEdgeMode}
            onClick={() => setMode('edge')}
          >
            <EdgeIcon />
            <span className={styles.modeLabel}>
              <span className={styles.modeName}>Edge</span>
              <span className={styles.modeHint}>traffic between</span>
            </span>
          </button>
          <button
            type="button"
            role="tab"
            aria-selected={isCatalogMode}
            className={styles.modeButton}
            data-active={isCatalogMode}
            onClick={() => setMode('catalog')}
          >
            <CatalogIcon />
            <span className={styles.modeLabel}>
              <span className={styles.modeName}>Catalog</span>
              <span className={styles.modeHint}>all metrics</span>
            </span>
          </button>
        </div>
      </header>

      {isCatalogMode ? (
        <MetricCatalogTable onOpenInExplorer={handleOpenInExplorer} />
      ) : isEntityMode ? (
        <>
          <GlobalEntitySearch nodes={nodes} onSelect={handleGlobalSelect} />
          <FilterBreadcrumbBuilder nodes={nodes} state={filter} setState={handleFilterChange} />
        </>
      ) : (
        <EdgeSelector
          nodes={nodes}
          edges={edges}
          selectedSource={edgeSourceId}
          selectedTarget={edgeTargetId}
          onSelect={handleEdgeSelect}
        />
      )}

      {!isCatalogMode && isEntityMode && selectedNode && (
        <SelectionSummary
          kind="entity"
          node={selectedNode}
          timeRange={timeRange}
          onTimeRangeChange={setTimeRange}
        />
      )}
      {!isCatalogMode && !isEntityMode && edgeSourceId && edgeTargetId && (
        <SelectionSummary
          kind="edge"
          source={nodeById.get(edgeSourceId) ?? null}
          target={nodeById.get(edgeTargetId) ?? null}
          relation="calls"
          timeRange={timeRange}
          onTimeRangeChange={setTimeRange}
        />
      )}

      {!isCatalogMode && !hasSelection && (
        <div className={styles.prompt}>
          {isEntityMode ? (
            <>
              <span>Select an entity to view its metrics</span>
              <span className={styles.promptHint}>
                Press <kbd className={styles.kbd}>⌘K</kbd> to search, or drill down by Namespace → Kind → Entity.
              </span>
            </>
          ) : (
            <>
              <span>Select a graph edge to view edge metrics</span>
              <span className={styles.promptHint}>
                Search by source or target name, or open the dropdown to browse <code>calls</code> edges.
              </span>
            </>
          )}
        </div>
      )}

      {!isCatalogMode && hasSelection && visibleCategories.length > 0 && (
        <CategoryTabs active={category} categories={visibleCategories} onChange={setCategory} />
      )}

      {!isCatalogMode && hasSelection && visibleCategories.length === 0 && (
        <div className={styles.prompt}>
          No metric categories apply to {selectedKind ?? 'this selection'}
        </div>
      )}

      {!isCatalogMode && hasSelection && visibleCategories.length > 0 && categoryMetrics.length > 0 && (
        <div className={styles.chartGrid}>
          {categoryMetrics.map((metric) => {
            const series = seriesByMetric.get(metric.name)
            return (
              <MetricCard
                key={metric.name}
                name={metric.display_name}
                canonicalName={metric.name}
                unit={metric.unit}
                samples={series?.samples ?? []}
                sources={metric.sources}
                syncId={SYNCED_CURSOR_ID}
              />
            )
          })}
        </div>
      )}

      {!isCatalogMode && hasSelection && visibleCategories.length > 0 && categoryMetrics.length === 0 && (
        <div className={styles.prompt}>No metrics defined for this category</div>
      )}

      {!isCatalogMode && allSamples.length > 0 && (
        <RawSamplesTable
          samples={allSamples}
          scopeFor={(sample) => {
            if (sample.edge) {
              const s = nodeById.get(sample.edge.source_id)
              const t = nodeById.get(sample.edge.target_id)
              return `${s?.name ?? sample.edge.source_id} → ${t?.name ?? sample.edge.target_id}`
            }
            const node = nodeById.get(sample.entity_id)
            if (!node) return sample.entity_id
            return `${node.namespace ? `${node.namespace}/` : ''}${node.name}`
          }}
        />
      )}
    </div>
  )
}

function EntityIcon() {
  return (
    <svg
      className={styles.modeIcon}
      width={20}
      height={20}
      viewBox="0 0 20 20"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.6}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <circle cx="10" cy="10" r="3.2" fill="currentColor" fillOpacity="0.18" />
      <circle cx="10" cy="10" r="3.2" />
      <circle cx="10" cy="10" r="7.5" strokeOpacity="0.45" strokeDasharray="2 2.5" />
    </svg>
  )
}

function CatalogIcon() {
  return (
    <svg
      className={styles.modeIcon}
      width={20}
      height={20}
      viewBox="0 0 20 20"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.6}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <rect x="3" y="3.5" width="14" height="13" rx="1.5" />
      <path d="M3 7.5h14" />
      <path d="M3 11.5h14" />
      <path d="M7.5 3.5v13" />
    </svg>
  )
}

function EdgeIcon() {
  return (
    <svg
      className={styles.modeIcon}
      width={20}
      height={20}
      viewBox="0 0 20 20"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.6}
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden
    >
      <circle cx="4.5" cy="10" r="2.3" fill="currentColor" fillOpacity="0.18" />
      <circle cx="4.5" cy="10" r="2.3" />
      <circle cx="15.5" cy="10" r="2.3" fill="currentColor" fillOpacity="0.18" />
      <circle cx="15.5" cy="10" r="2.3" />
      <path d="M7 10h6" />
      <path d="M11.5 8l1.8 2-1.8 2" />
    </svg>
  )
}
