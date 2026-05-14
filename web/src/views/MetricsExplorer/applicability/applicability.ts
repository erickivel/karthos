import type { EntityKind } from '@/types/graph'
import type { CanonicalMetric, MetricCategory, MetricScope } from '@/types/metrics'

export type ExplorerMode = 'entity' | 'edge'

const ALL_CATEGORIES: MetricCategory[] = ['resource', 'traffic', 'latency', 'error', 'capacity', 'health']
const EDGE_CATEGORIES: MetricCategory[] = ['traffic', 'latency', 'error']
const WORKLOAD_TYPE_LABEL = 'k8s.workload.type'

interface ApplicabilityContext {
  labels?: Record<string, string>
}

const WORKLOAD_METRIC_PREFIX_BY_TYPE: Record<string, string> = {
  deployment: 'k8s.deployment.',
  statefulset: 'k8s.statefulset.',
  daemonset: 'k8s.daemonset.',
  replicaset: 'k8s.replicaset.',
  job: 'k8s.job.',
  cronjob: 'k8s.cronjob.',
  replication_controller: 'k8s.replication_controller.',
}

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
  context?: ApplicabilityContext,
): MetricCategory[] {
  if (!metrics) return mode === 'edge' ? EDGE_CATEGORIES : ALL_CATEGORIES
  if (mode === 'entity' && kind === null) return ALL_CATEGORIES

  const visible = ALL_CATEGORIES.filter((category) =>
    applicableMetrics(metrics, category, kind, mode, context).length > 0,
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
  context?: ApplicabilityContext,
): boolean {
  if (kind === null) return true
  if (!metric.applies_to) {
    warnMissingAppliesToOnce(metric.name)
    return true
  }
  if (kind === 'workload' && metric.applies_to.includes(kind) && !isMetricApplicableToWorkloadType(metric, context)) {
    return false
  }
  return metric.applies_to.includes(kind)
}

function isMetricApplicableToWorkloadType(metric: CanonicalMetric, context?: ApplicabilityContext): boolean {
  const workloadType = context?.labels?.[WORKLOAD_TYPE_LABEL]
  if (!workloadType) return true
  const expectedPrefix = WORKLOAD_METRIC_PREFIX_BY_TYPE[workloadType]
  if (!expectedPrefix) return true
  if (!isControllerWorkloadMetric(metric.name)) return true
  return metric.name.startsWith(expectedPrefix)
}

function isControllerWorkloadMetric(metricName: string): boolean {
  return Object.values(WORKLOAD_METRIC_PREFIX_BY_TYPE).some((prefix) => metricName.startsWith(prefix))
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
  context?: ApplicabilityContext,
): CanonicalMetric[] {
  const requiredScope: MetricScope = mode === 'entity' ? 'entity' : 'edge'
  return (metrics ?? []).filter((metric) => {
    if (metric.category !== category) return false
    if (!metric.scopes.includes(requiredScope)) return false
    if (mode === 'entity' && !isMetricApplicableToKind(metric, kind, context)) return false
    return true
  })
}

/** Test-only helper to reset the dev-warn dedupe set. */
export function __resetApplicabilityWarnings(): void {
  warnedMetrics.clear()
}
