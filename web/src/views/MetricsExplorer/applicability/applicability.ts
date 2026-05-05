import type { EntityKind } from '@/types/graph'
import type { CanonicalMetric, MetricCategory, MetricScope } from '@/types/metrics'
import { KIND_CATEGORIES } from './kindCategories'
import { SOURCE_KINDS } from './sourceKinds'

export type ExplorerMode = 'entity' | 'edge'

const ALL_CATEGORIES: MetricCategory[] = ['resource', 'traffic', 'latency', 'error', 'capacity', 'health']
const EDGE_CATEGORIES: MetricCategory[] = ['traffic', 'latency', 'error']

const warnedSources = new Set<string>()

function warnUnknownSourceOnce(sourceId: string): void {
  if (!import.meta.env.DEV) return
  if (warnedSources.has(sourceId)) return
  warnedSources.add(sourceId)
  console.warn(
    `[applicability] Unknown source id "${sourceId}". ` +
      `Add it to web/src/views/MetricsExplorer/applicability/sourceKinds.ts. ` +
      `Treating as applicable to every kind (fail-open).`,
  )
}

/**
 * Returns the metric categories (tabs) that should be visible for a given
 * entity kind in the given explorer mode.
 *
 * In edge mode the kind argument is ignored; edges always expose
 * traffic/latency/error.
 *
 * If `kind` is null (no entity selected, or selection has mixed kinds), the
 * union of all categories is returned so the user still sees every tab.
 */
export function visibleCategoriesForKind(
  kind: EntityKind | null,
  mode: ExplorerMode,
): MetricCategory[] {
  if (mode === 'edge') return EDGE_CATEGORIES
  if (kind === null) return ALL_CATEGORIES
  return KIND_CATEGORIES[kind] ?? ALL_CATEGORIES
}

/**
 * Returns true if any of the metric's catalog sources can emit data for the
 * given entity kind. Unknown source ids fail open and warn once in dev.
 *
 * A metric with no declared sources (rare) is treated as applicable.
 */
export function isMetricApplicableToKind(
  metric: CanonicalMetric,
  kind: EntityKind | null,
): boolean {
  if (kind === null) return true
  if (!metric.sources || metric.sources.length === 0) return true
  return metric.sources.some((source) => {
    const mapping = SOURCE_KINDS[source.id]
    if (mapping === undefined) {
      warnUnknownSourceOnce(source.id)
      return true
    }
    if (mapping === '*') return true
    return mapping.includes(kind)
  })
}

/**
 * Composes scope + category + applicability filtering. The frontend single
 * source of truth for "which metrics should I render now".
 */
export function applicableMetrics(
  metrics: CanonicalMetric[] | undefined,
  category: MetricCategory,
  kind: EntityKind | null,
  mode: ExplorerMode,
): CanonicalMetric[] {
  const requiredScope: MetricScope = mode === 'entity' ? 'entity' : 'edge'
  return (metrics ?? []).filter((metric) => {
    if (metric.category !== category) return false
    if (!metric.scopes.includes(requiredScope)) return false
    if (mode === 'entity' && !isMetricApplicableToKind(metric, kind)) return false
    return true
  })
}

/** Test-only helper to reset the dev-warn dedupe set. */
export function __resetApplicabilityWarnings(): void {
  warnedSources.clear()
}
