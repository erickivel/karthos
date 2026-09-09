import type { MetricSeries } from '@/types/metrics'
import { latestSample } from '@/utils/metricSeries'

function titleCase(value: string) {
  return value.split('_').map((part) => part.charAt(0).toUpperCase() + part.slice(1)).join(' ')
}

/**
 * Enum integers are deliberately translated at the API boundary. A missing
 * label is therefore a contract problem, not Kubernetes reporting Unknown.
 */
export function podEnumLabel(series?: MetricSeries): string {
  const sample = latestSample(series?.samples)
  if (!sample) return 'No current sample'
  if (!sample.value_label) return 'No readable API label'
  return titleCase(sample.value_label)
}
