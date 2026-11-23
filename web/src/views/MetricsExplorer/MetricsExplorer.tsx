import { useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { useQueries, useQuery } from '@tanstack/react-query'
import { ChevronDown, ListTree } from 'lucide-react'
import { useMetricCatalog } from '@/hooks/useMetricCatalog'
import { useGraphSnapshot } from '@/hooks/useGraphSnapshot'
import { getAllSeries, getSeries } from '@/api/metrics'
import { getTimeRange, type TimeRange } from './TimeRangeSelector'
import { CategoryTabs } from './CategoryTabs'
import { MetricCard } from './MetricCard'
import { NodeCpuSummary } from './NodeCpuSummary'
import { PodHealthSummary } from './PodHealthSummary'
import { PodVolumeList } from './PodVolumeList'
import { ContainerOverview } from './ContainerOverview'
import { PVCOverview } from './PVCOverview'
import { deriveNodeCpuUtilization } from './nodeCpuDerivation'
import { ServiceTrafficSummary } from './ServiceTrafficSummary'
import { MetricLoadingSkeleton } from './MetricLoadingSkeleton'
import { RawSamplesTable } from './RawSamplesTable'
import { SelectionSummary } from './SelectionSummary'
import { GlobalEntitySearch } from './filter/GlobalEntitySearch'
import { FilterBreadcrumbBuilder } from './filter/FilterBreadcrumbBuilder'
import { EdgeSelector } from './edge/EdgeSelector'
import { hasCurrentCallsEdge } from './model'
import { EMPTY_FILTER, type FilterState } from './filter/types'
import { applicableMetrics, visibleCategoriesForSelection, type ExplorerMode } from './applicability/applicability'
import { MetricCatalogTable } from './catalog/MetricCatalogTable'
import { metricsForNodeResourceSection, NODE_RESOURCE_SECTIONS, type NodeResourceSection } from './nodeResourceSections'
import { metricsForPodSection, POD_METRIC_SECTIONS, type PodMetricSection } from './podMetricSections'
import { groupPodVolumeSeries } from './podVolumeModel'
import { buildContainerOverview, CONTAINER_METRIC_NAMES, type ContainerMetricName, type ContainerSeriesByMetric } from './containerOverviewModel'
import { buildPVCOverview, PVC_METRIC_NAMES, type PVCMetricName, type PVCSeriesByMetric } from './pvcOverviewModel'
import { metricPresentation } from './metricPresentation'
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
  const [seriesPageTokens, setSeriesPageTokens] = useState<Record<string, string[]>>({})
  const [cpuBreakdownOpen, setCpuBreakdownOpen] = useState(false)
  const [nodeResourceSection, setNodeResourceSection] = useState<NodeResourceSection>('compute')
  const [podMetricSection, setPodMetricSection] = useState<PodMetricSection>('overview')

  const { data: catalog, isLoading: catalogLoading } = useMetricCatalog()
  const { data: snapshot, isLoading: snapshotLoading } = useGraphSnapshot()
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
  const visibleCategories = useMemo(
    () => visibleCategoriesForSelection(catalog?.metrics, isEntityMode ? selectedKind : null, mode),
    [catalog?.metrics, isEntityMode, selectedKind, mode],
  )

  const isPodPresentation = isEntityMode && selectedKind === 'pod'
  const isContainerPresentation = isEntityMode && selectedKind === 'container'
  const isPVCPresentation = isEntityMode && selectedKind === 'pvc'
  const podApplicableMetrics = useMemo(
    () => (catalog?.metrics ?? []).filter((metric) => metric.scopes.includes('entity') && metric.applies_to?.includes('pod')),
    [catalog?.metrics],
  )
  const containerApplicableMetrics = useMemo(
    () => (catalog?.metrics ?? []).filter((metric) => metric.scopes.includes('entity') && metric.applies_to?.includes('container')),
    [catalog?.metrics],
  )
  const pvcApplicableMetrics = useMemo(
    () => (catalog?.metrics ?? []).filter((metric) => metric.scopes.includes('entity') && metric.applies_to?.includes('pvc')),
    [catalog?.metrics],
  )

  const categoryMetrics = useMemo(
    () => isPodPresentation
      ? podApplicableMetrics
      : isContainerPresentation
        ? containerApplicableMetrics
        : isPVCPresentation
          ? pvcApplicableMetrics
        : applicableMetrics(
        catalog?.metrics,
        category,
        isEntityMode ? selectedKind : null,
        mode,
      ),
    [catalog?.metrics, category, mode, selectedKind, isEntityMode, isPodPresentation, isContainerPresentation, isPVCPresentation, podApplicableMetrics, containerApplicableMetrics, pvcApplicableMetrics],
  )

  const isNodeResourcePresentation = isEntityMode && selectedKind === 'node' && category === 'resource'
  const isNodeCpuPresentation = isNodeResourcePresentation && nodeResourceSection === 'compute'
  const isPodHealthPresentation = isPodPresentation && podMetricSection === 'health'
  const isPodVolumePresentation = isPodPresentation && podMetricSection === 'volumes'
  const isServiceTrafficPresentation = isEntityMode && selectedKind === 'service' && category === 'traffic'
  const serviceTrafficMetric = isServiceTrafficPresentation
    ? categoryMetrics.find((metric) => metric.name === 'servicegraph.request.count')
    : undefined
  const displayedCategoryMetrics = useMemo(() => {
    if (isPodPresentation) {
      const allowed = new Set(metricsForPodSection(categoryMetrics.map((metric) => metric.name), podMetricSection))
      return categoryMetrics.filter((metric) => allowed.has(metric.name))
    }
    if (isContainerPresentation) return categoryMetrics
    if (isPVCPresentation) return categoryMetrics
    if (!isNodeResourcePresentation) return categoryMetrics
    const metricNames = metricsForNodeResourceSection(categoryMetrics.map((metric) => metric.name), nodeResourceSection)
    const allowed = new Set(metricNames)
    return categoryMetrics.filter((metric) => allowed.has(metric.name))
  }, [categoryMetrics, isNodeResourcePresentation, isPodPresentation, isContainerPresentation, isPVCPresentation, nodeResourceSection, podMetricSection])
  // CPU state series are fetched as one complete cursor walk for the summary
  // and expandable breakdown. Service graph request series are likewise fetched
  // as complete cursor walks before deriving source-scoped inbound and outbound
  // summaries without mixing their directional evidence.
  const queriedMetrics = useMemo(
    () => (isPodHealthPresentation || isPodVolumePresentation || isContainerPresentation || isPVCPresentation ? [] : displayedCategoryMetrics.filter((metric) => (
      !(isNodeCpuPresentation && metric.name === 'system.cpu.utilization')
      && metric.name !== serviceTrafficMetric?.name
    ))),
    [displayedCategoryMetrics, isNodeCpuPresentation, isPodHealthPresentation, isPodVolumePresentation, isContainerPresentation, isPVCPresentation, serviceTrafficMetric?.name],
  )

  const podHealthMetrics = useMemo(
    () => isPodHealthPresentation ? displayedCategoryMetrics : [],
    [displayedCategoryMetrics, isPodHealthPresentation],
  )
  const podVolumeMetrics = useMemo(
    () => isPodVolumePresentation ? displayedCategoryMetrics : [],
    [displayedCategoryMetrics, isPodVolumePresentation],
  )
  const containerMetrics = useMemo(
    () => isContainerPresentation
      ? CONTAINER_METRIC_NAMES.flatMap((name) => {
          const metric = categoryMetrics.find((candidate) => candidate.name === name)
          return metric ? [metric] : []
        })
      : [],
    [categoryMetrics, isContainerPresentation],
  )
  const pvcMetrics = useMemo(
    () => isPVCPresentation
      ? PVC_METRIC_NAMES.flatMap((name) => {
          const metric = categoryMetrics.find((candidate) => candidate.name === name)
          return metric ? [metric] : []
        })
      : [],
    [categoryMetrics, isPVCPresentation],
  )

  const selectedCurrentCallsEdge = useMemo(
    () => hasCurrentCallsEdge(edges, edgeSourceId, edgeTargetId),
    [edges, edgeSourceId, edgeTargetId],
  )
  const staleEdgeSelection = isEdgeMode
    && Boolean(edgeSourceId && edgeTargetId)
    && snapshot !== undefined
    && !selectedCurrentCallsEdge
  const hasSelection = isEntityMode
    ? Boolean(selectedNode)
    : isEdgeMode
      ? selectedCurrentCallsEdge
      : false
  const awaitingEntityContext = isEntityMode
    && Boolean(entityId)
    && (catalogLoading || snapshotLoading)

  // Snap to a still-visible category whenever the visibility set changes.
  useEffect(() => {
    if (visibleCategories.length === 0) return
    if (!visibleCategories.includes(category)) {
      setCategory(visibleCategories[0]!)
    }
  }, [category, visibleCategories])

  const seriesQueries = useQueries({
    queries: queriedMetrics.map((metric) => ({
      queryKey: [
        'metrics',
        mode,
        metric.name,
        isEntityMode ? filter.entityId : '',
        edgeSourceId,
        edgeTargetId,
        from,
        to,
        seriesPageTokens[metric.name]?.[seriesPageTokens[metric.name]!.length - 1] ?? '',
      ],
      queryFn: () =>
        isEntityMode
          ? getSeries({
              metric_name: metric.name,
              entity_id: [filter.entityId],
              from,
              to,
              page_size: 50,
              next_token: seriesPageTokens[metric.name]?.[seriesPageTokens[metric.name]!.length - 1],
            })
          : getSeries({
              metric_name: metric.name,
              edge_source_id: edgeSourceId,
              edge_target_id: edgeTargetId,
              from,
              to,
              page_size: 50,
              next_token: seriesPageTokens[metric.name]?.[seriesPageTokens[metric.name]!.length - 1],
            }),
      enabled: !isCatalogMode && hasSelection,
      refetchInterval: 30_000,
    })),
  })

  const cpuBreakdownQuery = useQuery({
    queryKey: ['node-cpu-breakdown', filter.entityId, from, to],
    queryFn: () => getAllSeries({
      metric_name: 'system.cpu.utilization',
      entity_id: [filter.entityId],
      from,
      to,
      page_size: 50,
    }),
    enabled: isNodeCpuPresentation && cpuBreakdownOpen && Boolean(filter.entityId),
    refetchInterval: 30_000,
  })

  const podHealthQueries = useQueries({
    queries: podHealthMetrics.map((metric) => ({
      queryKey: ['pod-health', metric.name, filter.entityId, from, to],
      queryFn: () => getSeries({
        metric_name: metric.name,
        entity_id: [filter.entityId],
        from,
        to,
        page_size: 50,
      }),
      enabled: Boolean(filter.entityId),
      refetchInterval: 30_000,
    })),
  })

  const podVolumeQueries = useQueries({
    queries: podVolumeMetrics.map((metric) => ({
      queryKey: ['pod-volumes', metric.name, filter.entityId, from, to],
      queryFn: () => getAllSeries({
        metric_name: metric.name,
        entity_id: [filter.entityId],
        from,
        to,
        page_size: 50,
      }),
      enabled: Boolean(filter.entityId),
      refetchInterval: 30_000,
    })),
  })

  const containerQueries = useQueries({
    queries: containerMetrics.map((metric) => ({
      queryKey: ['container-overview', metric.name, filter.entityId, from, to],
      queryFn: () => getAllSeries({
        metric_name: metric.name,
        entity_id: [filter.entityId],
        from,
        to,
        page_size: 50,
      }),
      enabled: Boolean(filter.entityId),
      refetchInterval: 30_000,
    })),
  })

  const pvcQueries = useQueries({
    queries: pvcMetrics.map((metric) => ({
      queryKey: ['pvc-overview', metric.name, filter.entityId, from, to],
      queryFn: () => getAllSeries({
        metric_name: metric.name,
        entity_id: [filter.entityId],
        from,
        to,
        page_size: 50,
      }),
      enabled: Boolean(filter.entityId),
      refetchInterval: 30_000,
    })),
  })

  const serviceTrafficOutboundQuery = useQuery({
    queryKey: ['service-traffic-summary', 'outbound', serviceTrafficMetric?.name, filter.entityId, from, to],
    queryFn: () => getAllSeries({
      metric_name: serviceTrafficMetric!.name,
      edge_source_id: filter.entityId,
      from,
      to,
      page_size: 50,
    }),
    enabled: Boolean(serviceTrafficMetric && filter.entityId),
    refetchInterval: 30_000,
  })

  const serviceTrafficInboundQuery = useQuery({
    queryKey: ['service-traffic-summary', 'inbound', serviceTrafficMetric?.name, filter.entityId, from, to],
    queryFn: () => getAllSeries({
      metric_name: serviceTrafficMetric!.name,
      edge_target_id: filter.entityId,
      from,
      to,
      page_size: 50,
    }),
    enabled: Boolean(serviceTrafficMetric && filter.entityId),
    refetchInterval: 30_000,
  })

  useEffect(() => {
    setSeriesPageTokens({})
  }, [mode, filter.entityId, edgeSourceId, edgeTargetId, from, to])

  const visibleSeries = useMemo(() => {
    const rows: { metric: CanonicalMetric; series: MetricSeries; queryIndex: number }[] = []
    queriedMetrics.forEach((metric, queryIndex) => {
      for (const series of seriesQueries[queryIndex]?.data?.series ?? []) {
        if (isNodeCpuPresentation && metric.name === 'k8s.node.cpu.usage') continue
        rows.push({ metric, series, queryIndex })
      }
    })
    return rows
  }, [queriedMetrics, seriesQueries, isNodeCpuPresentation])

  const directNodeCpuQueryIndex = queriedMetrics.findIndex((metric) => metric.name === 'k8s.node.cpu.usage')
  const directNodeCpuResponse = directNodeCpuQueryIndex >= 0 ? seriesQueries[directNodeCpuQueryIndex]?.data : undefined
  const nodeCpuMetric = categoryMetrics.find((metric) => metric.name === 'k8s.node.cpu.usage')
  const cpuBreakdownMetric = categoryMetrics.find((metric) => metric.name === 'system.cpu.utilization')
  const derivedNodeCpu = useMemo(
    () => deriveNodeCpuUtilization(cpuBreakdownQuery.data?.series ?? []),
    [cpuBreakdownQuery.data?.series],
  )

  const emptyNodeMetrics = useMemo(() => {
    if (!isEntityMode || selectedKind !== 'node') return []
    return queriedMetrics.flatMap((metric, queryIndex) => {
      // The dedicated CPU summary owns the direct kubelet empty state too.
      if (isNodeCpuPresentation && metric.name === 'k8s.node.cpu.usage') return []
      const response = seriesQueries[queryIndex]?.data
      return response && response.series.length === 0 ? [{ metric, response }] : []
    })
  }, [isEntityMode, isNodeCpuPresentation, queriedMetrics, selectedKind, seriesQueries])

  const metricsLoading = seriesQueries.some((query) => query.isLoading)
    || podHealthQueries.some((query) => query.isLoading)
    || podVolumeQueries.some((query) => query.isLoading)
    || containerQueries.some((query) => query.isLoading)
    || pvcQueries.some((query) => query.isLoading)

  const podHealthByName = new Map(podHealthMetrics.map((metric, index) => [metric.name, podHealthQueries[index]?.data]))
  const podVolumeGroups = useMemo(() => groupPodVolumeSeries(Object.fromEntries(
    podVolumeMetrics.map((metric, index) => [metric.name, podVolumeQueries[index]?.data?.series ?? []]),
  )), [podVolumeMetrics, podVolumeQueries])
  const containerSeriesByMetric = useMemo(() => Object.fromEntries(
    containerMetrics.map((metric, index) => [metric.name, containerQueries[index]?.data?.series ?? []]),
  ) as ContainerSeriesByMetric, [containerMetrics, containerQueries])
  const containerOverview = useMemo(
    () => buildContainerOverview(selectedNode?.status, containerSeriesByMetric),
    [containerSeriesByMetric, selectedNode?.status],
  )
  const containerMetricsByName = useMemo(() => Object.fromEntries(
    containerMetrics.map((metric) => [metric.name, metric]),
  ) as Partial<Record<ContainerMetricName, CanonicalMetric>>, [containerMetrics])
  const containerQueriesByName = useMemo(() => Object.fromEntries(
    containerMetrics.flatMap((metric, index) => {
      const query = containerQueries[index]?.data?.query
      return query ? [[metric.name, query] as const] : []
    }),
  ) as Partial<Record<ContainerMetricName, NonNullable<(typeof containerQueries)[number]['data']>['query']>>, [containerMetrics, containerQueries])
  const containerDetailSeries = useMemo(() => containerMetrics.flatMap((metric, index) => {
    const response = containerQueries[index]?.data
    return response && response.series.length > 1
      ? response.series.map((series) => ({ metric, series, query: response.query }))
      : []
  }), [containerMetrics, containerQueries])
  const pvcSeriesByMetric = useMemo(() => Object.fromEntries(
    pvcMetrics.map((metric, index) => [metric.name, pvcQueries[index]?.data?.series ?? []]),
  ) as PVCSeriesByMetric, [pvcMetrics, pvcQueries])
  const pvcOverview = useMemo(
    () => buildPVCOverview(pvcSeriesByMetric),
    [pvcSeriesByMetric],
  )
  const pvcMetricsByName = useMemo(() => Object.fromEntries(
    pvcMetrics.map((metric) => [metric.name, metric]),
  ) as Partial<Record<PVCMetricName, CanonicalMetric>>, [pvcMetrics])
  const pvcQueriesByName = useMemo(() => Object.fromEntries(
    pvcMetrics.flatMap((metric, index) => {
      const query = pvcQueries[index]?.data?.query
      return query ? [[metric.name, query] as const] : []
    }),
  ) as Partial<Record<PVCMetricName, NonNullable<(typeof pvcQueries)[number]['data']>['query']>>, [pvcMetrics, pvcQueries])

  const allSamples = useMemo(() => {
    const out: MetricSample[] = []
    for (const { series } of visibleSeries) {
      out.push(...series.samples)
    }
    for (const series of serviceTrafficOutboundQuery.data?.series ?? []) {
      out.push(...series.samples)
    }
    for (const series of serviceTrafficInboundQuery.data?.series ?? []) {
      out.push(...series.samples)
    }
    for (const response of podHealthQueries) {
      for (const series of response.data?.series ?? []) out.push(...series.samples)
    }
    for (const response of podVolumeQueries) {
      for (const series of response.data?.series ?? []) out.push(...series.samples)
    }
    for (const response of containerQueries) {
      for (const series of response.data?.series ?? []) out.push(...series.samples)
    }
    for (const response of pvcQueries) {
      for (const series of response.data?.series ?? []) out.push(...series.samples)
    }
    return out
  }, [visibleSeries, serviceTrafficInboundQuery.data?.series, serviceTrafficOutboundQuery.data?.series, podHealthQueries, podVolumeQueries, containerQueries, pvcQueries])

  const changeSeriesPage = (metricName: string, nextToken?: string, direction: 'next' | 'previous' = 'next') => {
    setSeriesPageTokens((current) => {
      const stack = current[metricName] ?? []
      if (direction === 'previous') {
        return { ...current, [metricName]: stack.slice(0, -1) }
      }
      return nextToken ? { ...current, [metricName]: [...stack, nextToken] } : current
    })
  }

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
      {!isCatalogMode && isEdgeMode && selectedCurrentCallsEdge && (
        <SelectionSummary
          kind="edge"
          source={nodeById.get(edgeSourceId) ?? null}
          target={nodeById.get(edgeTargetId) ?? null}
          relation="calls"
          timeRange={timeRange}
          onTimeRangeChange={setTimeRange}
        />
      )}

      {!isCatalogMode && staleEdgeSelection && (
        <div className={styles.prompt}>
          <span>This connection is not in the current graph snapshot</span>
          <span className={styles.promptHint}>
            The explorer only shows traffic for active <code>calls</code> edges. Choose a current edge above.
          </span>
        </div>
      )}

      {awaitingEntityContext && (
        <MetricLoadingSkeleton count={2} label="Loading selected entity metrics" />
      )}

      {!isCatalogMode && !hasSelection && !staleEdgeSelection && !awaitingEntityContext && (
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

      {!isCatalogMode && hasSelection && visibleCategories.length > 0 && !isPodPresentation && !isContainerPresentation && !isPVCPresentation && (
        <CategoryTabs active={category} categories={visibleCategories} onChange={setCategory} />
      )}

      {isNodeResourcePresentation && (
        <div className={styles.resourceSections} aria-label="Node resource sections">
          {NODE_RESOURCE_SECTIONS.map((section) => (
            <button
              key={section.id}
              type="button"
              className={styles.resourceSection}
              data-active={nodeResourceSection === section.id}
              onClick={() => setNodeResourceSection(section.id)}
            >
              {section.label}
            </button>
          ))}
        </div>
      )}

      {isPodPresentation && (
        <div className={styles.resourceSections} aria-label="Pod metric sections">
          {POD_METRIC_SECTIONS.map((section) => (
            <button
              key={section.id}
              type="button"
              className={styles.resourceSection}
              data-active={podMetricSection === section.id}
              onClick={() => setPodMetricSection(section.id)}
            >
              {section.label}
            </button>
          ))}
        </div>
      )}

      {!isCatalogMode && hasSelection && visibleCategories.length === 0 && (
        <div className={styles.prompt}>
          No metric categories apply to {selectedKind ?? 'this selection'}
        </div>
      )}

      {!isCatalogMode && hasSelection && visibleCategories.length > 0 && categoryMetrics.length > 0 && (
        <div className={styles.chartGrid}>
          {metricsLoading && visibleSeries.length === 0 && (
            <MetricLoadingSkeleton
              count={isNodeCpuPresentation || isContainerPresentation || isPVCPresentation ? 1 : Math.min(3, Math.max(1, queriedMetrics.length))}
              fullWidth={isNodeCpuPresentation || isContainerPresentation || isPVCPresentation}
              label={`Loading ${isNodeResourcePresentation ? nodeResourceSection : isPodPresentation ? podMetricSection : isContainerPresentation ? 'container overview' : isPVCPresentation ? 'PVC storage' : category} metrics`}
            />
          )}
          {isNodeCpuPresentation && directNodeCpuResponse && nodeCpuMetric && (
            <NodeCpuSummary
              directSeries={directNodeCpuResponse.series[0]}
              derived={derivedNodeCpu}
              sources={nodeCpuMetric.sources}
            />
          )}
          {isNodeCpuPresentation && cpuBreakdownMetric && (
            <section className={styles.cpuBreakdown} aria-label="CPU per-core and state breakdown">
              <div className={styles.cpuBreakdownHeader}>
                <div>
                  <span className={styles.cpuBreakdownLabel}>CPU breakdown</span>
                  <span className={styles.cpuBreakdownHint}>{cpuBreakdownQuery.data?.total ?? 'Per-core/state data loads when opened'} {cpuBreakdownQuery.data ? 'per-core/state series' : ''}</span>
                </div>
                <button
                  type="button"
                  className={styles.cpuBreakdownAction}
                  aria-expanded={cpuBreakdownOpen}
                  onClick={() => setCpuBreakdownOpen((open) => !open)}
                >
                  <ListTree size={15} aria-hidden="true" />
                  {cpuBreakdownOpen ? 'Hide per-core details' : 'Inspect per-core details'}
                  <ChevronDown size={15} aria-hidden="true" data-open={cpuBreakdownOpen} />
                </button>
              </div>
              {cpuBreakdownOpen && cpuBreakdownQuery.isLoading && (
                <MetricLoadingSkeleton count={2} label="Loading CPU per-core and state breakdown" />
              )}
              {cpuBreakdownOpen && cpuBreakdownQuery.data && <div className={styles.chartGrid}>
                {cpuBreakdownQuery.data.series.map((series, index) => (
                  <MetricCard
                    key={`cpu-breakdown:${index}:${series.samples[0]?.entity_id ?? ''}`}
                    name={cpuBreakdownMetric.display_name}
                    canonicalName={cpuBreakdownMetric.name}
                    unit={series.unit || cpuBreakdownMetric.unit}
                    samples={series.samples}
                    sources={cpuBreakdownMetric.sources}
                    query={cpuBreakdownQuery.data.query}
                    description={cpuBreakdownMetric.description}
                    scope="entity"
                    labels={series.samples[0]?.labels}
                    syncId={SYNCED_CURSOR_ID}
                  />
                ))}
              </div>}
            </section>
          )}
          {isPodHealthPresentation && !metricsLoading && (
            <PodHealthSummary
              phaseMetric={podHealthMetrics.find((metric) => metric.name === 'k8s.pod.status.phase')}
              reasonMetric={podHealthMetrics.find((metric) => metric.name === 'k8s.pod.status.reason')}
              phase={podHealthByName.get('k8s.pod.status.phase')?.series[0]}
              reason={podHealthByName.get('k8s.pod.status.reason')?.series[0]}
              query={podHealthByName.get('k8s.pod.status.phase')?.query ?? podHealthByName.get('k8s.pod.status.reason')?.query}
            />
          )}
          {isPodVolumePresentation && !metricsLoading && <PodVolumeList volumes={podVolumeGroups} />}
          {isContainerPresentation && !metricsLoading && (
            <ContainerOverview overview={containerOverview} metrics={containerMetricsByName} queries={containerQueriesByName} />
          )}
          {isPVCPresentation && !metricsLoading && (
            <PVCOverview overview={pvcOverview} metrics={pvcMetricsByName} queries={pvcQueriesByName} />
          )}
          {isContainerPresentation && containerDetailSeries.map(({ metric, series, query }, index) => (
            <MetricCard
              key={`container-detail:${metric.name}:${index}:${series.samples[0]?.entity_id ?? ''}`}
              name={metricPresentation({
                canonicalName: metric.name,
                scope: 'entity',
                fallbackTitle: metric.display_name,
                fallbackMeaning: metric.description,
              }).title}
              canonicalName={metric.name}
              unit={series.unit || metric.unit}
              samples={series.samples}
              sources={metric.sources}
              query={query}
              description={metric.description}
              scope="entity"
              labels={series.samples[0]?.labels}
              syncId={SYNCED_CURSOR_ID}
            />
          ))}
          {isServiceTrafficPresentation && serviceTrafficMetric && selectedNode && serviceTrafficInboundQuery.data && (
            <ServiceTrafficSummary
              metric={serviceTrafficMetric}
              series={serviceTrafficInboundQuery.data.series}
              query={serviceTrafficInboundQuery.data.query}
              direction="inbound"
              serviceName={selectedNode.name}
            />
          )}
          {isServiceTrafficPresentation && serviceTrafficMetric && selectedNode && serviceTrafficOutboundQuery.data && (
            <ServiceTrafficSummary
              metric={serviceTrafficMetric}
              series={serviceTrafficOutboundQuery.data.series}
              query={serviceTrafficOutboundQuery.data.query}
              direction="outbound"
              serviceName={selectedNode.name}
            />
          )}
          {!isPodHealthPresentation && !isPodVolumePresentation && !isContainerPresentation && !isPVCPresentation && visibleSeries.map(({ metric, series, queryIndex }, seriesIndex) => {
            const response = seriesQueries[queryIndex]?.data
            if (!response) return null
            return (
              <MetricCard
                key={`${metric.name}:${seriesIndex}:${series.samples[0]?.entity_id ?? ''}`}
                name={metric.display_name}
                canonicalName={metric.name}
                unit={series.unit || metric.unit}
                samples={series.samples}
                sources={metric.sources}
                query={response.query}
                description={metric.description}
                scope={isEntityMode ? 'entity' : 'edge'}
                labels={series.samples[0]?.labels}
                edge={series.edge ?? series.samples[0]?.edge}
                syncId={SYNCED_CURSOR_ID}
              />
            )
          })}
          {emptyNodeMetrics.map(({ metric, response }) => (
            <MetricCard
              key={`empty:${metric.name}`}
              name={metric.display_name}
              canonicalName={metric.name}
              unit={metric.unit}
              samples={[]}
              sources={metric.sources}
              query={response.query}
              description={metric.description}
              scope="entity"
              emptyMessage={`No current samples from ${metric.sources?.[0]?.id ?? 'the configured source'}`}
              syncId={SYNCED_CURSOR_ID}
            />
          ))}
        </div>
      )}

      {!isCatalogMode && hasSelection && visibleCategories.length > 0 && categoryMetrics.length > 0 && !isPodPresentation && !isContainerPresentation && !isPVCPresentation && (
        <div className={styles.seriesPagination}>
          {queriedMetrics.filter((metric) => !(isNodeCpuPresentation && metric.name === 'k8s.node.cpu.usage')).map((metric) => {
            const index = queriedMetrics.indexOf(metric)
            const response = seriesQueries[index]?.data
            if (!response) return null
            const page = seriesPageTokens[metric.name] ?? []
            return (
              <div key={metric.name} className={styles.seriesPager}>
                <span>{metric.display_name}: {response.series.length} of {response.total} series</span>
                {page.length > 0 && (
                  <button type="button" onClick={() => changeSeriesPage(metric.name, undefined, 'previous')}>Previous</button>
                )}
                {response.next_token && (
                  <button type="button" onClick={() => changeSeriesPage(metric.name, response.next_token)}>Next</button>
                )}
              </div>
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
