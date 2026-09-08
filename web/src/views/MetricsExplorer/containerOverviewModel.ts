import type { MetricSample, MetricSeries } from '@/types/metrics'

export const CONTAINER_METRIC_NAMES = [
  'k8s.container.cpu.usage',
  'k8s.container.cpu.request',
  'k8s.container.cpu.limit',
  'k8s.container.memory.usage',
  'k8s.container.memory.request',
  'k8s.container.memory.limit',
  'k8s.container.ready',
  'k8s.container.restarts',
] as const

export type ContainerMetricName = (typeof CONTAINER_METRIC_NAMES)[number]
export type ContainerSeriesByMetric = Partial<Record<ContainerMetricName, MetricSeries[]>>

export type ContainerMetricValue =
  | { kind: 'single'; value: number; unit: string; sample: MetricSample; series: MetricSeries; cumulative?: boolean }
  | { kind: 'multiple'; count: number; sampleCount: number }
  | { kind: 'not_reported' }

export interface ContainerOverview {
  status: string
  cpu: { used: ContainerMetricValue; request: ContainerMetricValue; limit: ContainerMetricValue }
  memory: { used: ContainerMetricValue; request: ContainerMetricValue; limit: ContainerMetricValue }
  restarts: ContainerMetricValue
}

function titleCase(value?: string) {
  if (!value) return 'Unknown'
  return value.split('_').map((part) => part.charAt(0).toUpperCase() + part.slice(1)).join(' ')
}

function latestSample(series: MetricSeries): MetricSample | undefined {
  const sorted = [...series.samples].sort((left, right) => (
    new Date(left.timestamp).getTime() - new Date(right.timestamp).getTime()
  ))
  return sorted[sorted.length - 1]
}

function metricValue(series: MetricSeries[] | undefined, cumulative = false): ContainerMetricValue {
  if (!series || series.length === 0) return { kind: 'not_reported' }
  if (series.length > 1) {
    return {
      kind: 'multiple',
      count: series.length,
      sampleCount: series.reduce((count, item) => count + item.samples.length, 0),
    }
  }
  const onlySeries = series[0]!
  const sample = latestSample(onlySeries)
  if (!sample) return { kind: 'not_reported' }
  return { kind: 'single', value: sample.value, unit: onlySeries.unit, sample, series: onlySeries, cumulative }
}

/**
 * Builds a no-aggregation Container projection. A summary value exists only
 * when the API returned exactly one current dimensional series.
 */
export function buildContainerOverview(status: string | undefined, seriesByMetric: ContainerSeriesByMetric): ContainerOverview {
  return {
    status: titleCase(status),
    cpu: {
      used: metricValue(seriesByMetric['k8s.container.cpu.usage']),
      request: metricValue(seriesByMetric['k8s.container.cpu.request']),
      limit: metricValue(seriesByMetric['k8s.container.cpu.limit']),
    },
    memory: {
      used: metricValue(seriesByMetric['k8s.container.memory.usage']),
      request: metricValue(seriesByMetric['k8s.container.memory.request']),
      limit: metricValue(seriesByMetric['k8s.container.memory.limit']),
    },
    restarts: metricValue(seriesByMetric['k8s.container.restarts'], true),
  }
}
