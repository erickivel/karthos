import { formatBytes, formatCpu, formatMem } from '@/utils/formatters'
import type { MetricSelector, NodeMetricSpec } from '../nodeMetrics'
import type { NodeMetricValues } from '@/hooks/useNodeMetrics'
import styles from './EntityNode.module.css'

interface NodeMetricProps {
  spec: NodeMetricSpec
  values: NodeMetricValues
  metadata?: Record<string, string>
}

function formatValue(format: 'cpu' | 'memory' | 'bytes' | 'percent' | undefined, v: number): string {
  switch (format) {
    case 'cpu':
      return formatCpu(v)
    case 'memory':
      return formatMem(v)
    case 'bytes':
      return formatBytes(v)
    case 'percent':
      return `${(v * 100).toFixed(0)}%`
    default:
      return v.toFixed(0)
  }
}

function utilizationTone(ratio: number): 'signal' | 'warn' | 'alert' {
  if (ratio >= 0.9) return 'alert'
  if (ratio >= 0.7) return 'warn'
  return 'signal'
}

function metricValue(values: NodeMetricValues, selector: MetricSelector | undefined): number | undefined {
  if (!selector) return undefined
  if (typeof selector === 'string') {
    return values[selector]
  }
  for (const metric of selector) {
    if (values[metric] !== undefined) return values[metric]
  }
  return undefined
}

function metadataNumber(metadata: Record<string, string> | undefined, key: string | undefined): number | undefined {
  if (!metadata || !key) return undefined
  const value = Number(metadata[key])
  return Number.isFinite(value) ? value : undefined
}

function MetricSegments({
  ratio,
  tone,
}: {
  ratio: number
  tone: 'signal' | 'warn' | 'alert' | 'info'
}) {
  const activeSegments = Math.ceil(Math.min(1, Math.max(0, ratio)) * 12)
  return (
    <span className={styles.metricSegments} data-tone={tone} aria-hidden="true">
      {Array.from({ length: 12 }, (_, index) => (
        <span key={index} data-active={index < activeSegments} />
      ))}
    </span>
  )
}

export function NodeMetric({ spec, values, metadata }: NodeMetricProps) {
  switch (spec.kind) {
    case 'utilization': {
      const raw = metricValue(values, spec.metric)
      if (raw === undefined) return null
      const capacity =
        metricValue(values, spec.capacityMetric) ??
        spec.capacityFallback
      if (!capacity || capacity <= 0) return null

      // When `invert` is set, the metric is "remaining" (e.g. volume.available)
      // and we render usage = capacity - remaining.
      const used = spec.invert ? Math.max(0, capacity - raw) : raw
      const ratio = Math.min(1, Math.max(0, used / capacity))
      const tone = utilizationTone(ratio)

      return (
        <div className={styles.metricBlock} data-tone={tone}>
          <div className={styles.metricHeader}>
            <span className={styles.metricLabel}>{spec.label}</span>
            <span className={styles.metricValue}>
              {formatValue(spec.format, used)}
            </span>
          </div>
          <MetricSegments ratio={ratio} tone={tone} />
        </div>
      )
    }
    case 'fraction': {
      const num = metricValue(values, spec.numeratorMetric)
      const den =
        metricValue(values, spec.denominatorMetric) ??
        metadataNumber(metadata, spec.denominatorMetadataKey)
      if (num === undefined || den === undefined) return null
      let tone: 'signal' | 'warn' | 'alert' = 'signal'
      if (num <= 0) tone = 'alert'
      else if (num < den) tone = 'warn'
      return (
        <div className={styles.metricBlock} data-tone={tone}>
          <div className={styles.metricHeader}>
            <span className={styles.metricLabel}>{spec.label}</span>
            <span className={styles.metricValue}>
              {Math.round(num)}/{Math.round(den)}
            </span>
          </div>
        </div>
      )
    }
    case 'rate': {
      const v = metricValue(values, spec.metric)
      if (v === undefined) return null
      const display = v >= 10 ? v.toFixed(0) : v.toFixed(1)
      return (
        <div className={styles.metricBlock} data-tone="info">
          <div className={styles.metricHeader}>
            <span className={styles.metricLabel}>{spec.label}</span>
            <span className={styles.metricValue}>
              {display} {spec.unit}
            </span>
          </div>
        </div>
      )
    }
    case 'gauge': {
      const v = metricValue(values, spec.metric)
      if (v === undefined) return null
      return (
        <div className={styles.metricBlock} data-tone="info">
          <div className={styles.metricHeader}>
            <span className={styles.metricLabel}>{spec.label}</span>
            <span className={styles.metricValue}>{formatValue(spec.format, v)}</span>
          </div>
        </div>
      )
    }
  }
}
