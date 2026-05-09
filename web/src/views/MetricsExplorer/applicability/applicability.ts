import type { EntityKind } from '@/types/graph'
import type { CanonicalMetric, MetricCategory, MetricScope } from '@/types/metrics'

export type ExplorerMode = 'entity' | 'edge'

const ALL_CATEGORIES: MetricCategory[] = ['resource', 'traffic', 'latency', 'error', 'capacity', 'health']
const EDGE_CATEGORIES: MetricCategory[] = ['traffic', 'latency', 'error']

const warnedMetrics = new Set<string>()

function warnMissingAppliesToOnce(metricName: string): void {
  if (!import.meta.env.DEV) return
  if (warnedMetrics.has(metricName)) return
  warnedMetrics.add(metricName)
  console.warn(
    `[applicability] Metric "${metricName}" has no applies_to catalog metadata. ` +
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
export function visibleCategoriesForSelection(
  metrics: CanonicalMetric[] | undefined,
  kind: EntityKind | null,
  mode: ExplorerMode,
): MetricCategory[] {
  if (!metrics) return mode === 'edge' ? EDGE_CATEGORIES : ALL_CATEGORIES
  if (mode === 'entity' && kind === null) return ALL_CATEGORIES

  const visible = ALL_CATEGORIES.filter((category) =>
    applicableMetrics(metrics, category, kind, mode).length > 0,
  )

  if (visible.length > 0) return visible
  if (mode === 'edge') return EDGE_CATEGORIES
  return []
}

/**
 * Returns true when the backend catalog says the metric applies to the given
 * entity kind. Missing metadata fails open for compatibility with older
 * backend responses, but current catalog tests require entity metrics to
 * declare this explicitly.
 */
export function isMetricApplicableToKind(
  metric: CanonicalMetric,
  kind: EntityKind | null,
): boolean {
  if (kind === null) return true
  if (!metric.applies_to) {
    warnMissingAppliesToOnce(metric.name)
    return true
  }
  return metric.applies_to.includes(kind)
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
  warnedMetrics.clear()
}
