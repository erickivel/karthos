import { Link } from 'react-router-dom'
import { formatCpu, formatMem } from '@/utils/formatters'
import { entityMetricsUrl } from '@/lib/links'
import type { NodeMetricSpec, MetricSelector } from '../nodeMetrics'
import type { NodeMetricValues, NodeMetricValue } from '@/hooks/useNodeMetrics'
import styles from './EntityNode.module.css'

interface NodeMetricProps {
  spec: NodeMetricSpec
  values: NodeMetricValues
  entityId: string
}

function valueFor(values: NodeMetricValues, selector: MetricSelector): NodeMetricValue | undefined {
  if (typeof selector === 'string') return values[selector]
  return selector.map((metric) => values[metric]).find((value) => value !== undefined)
}

function format(spec: NodeMetricSpec, value: number) {
  if (spec.format === 'cpu') return formatCpu(value)
  if (spec.format === 'memory') return formatMem(value)
  return `${value >= 10 ? value.toFixed(0) : value.toFixed(1)} rps`
}

export function NodeMetric({ spec, values, entityId }: NodeMetricProps) {
  const metric = valueFor(values, spec.metric)
  if (!metric) return null

  return (
    <div className={styles.metricBlock} data-tone="info">
      <div className={styles.metricHeader}>
        <span className={styles.metricLabel}>{spec.label}</span>
        {metric.seriesCount === 1 && metric.value !== undefined ? (
          <span className={styles.metricValue}>{format(spec, metric.value)}</span>
        ) : (
          <Link className={styles.metricValue} to={entityMetricsUrl(entityId)}>
            {metric.seriesCount} series
          </Link>
        )}
      </div>
    </div>
  )
}
