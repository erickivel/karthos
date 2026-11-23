import { Activity, CircleHelp, RotateCcw, Scale, ServerCog } from 'lucide-react'
import type { CanonicalMetric, MetricQuery } from '@/types/metrics'
import { formatMetricValue } from '@/utils/formatters'
import { InfoTooltip } from './InfoTooltip'
import { metricPresentation } from './metricPresentation'
import type { ContainerMetricName, ContainerMetricValue, ContainerOverview as ContainerOverviewModel } from './containerOverviewModel'
import styles from './ContainerOverview.module.css'

interface Props {
  overview: ContainerOverviewModel
  metrics: Partial<Record<ContainerMetricName, CanonicalMetric>>
  queries: Partial<Record<ContainerMetricName, MetricQuery>>
}

const fallbackQuery: MetricQuery = {
  aggregation: 'latest',
  from: '',
  to: '',
  step: '15s',
  timestamp_semantics: 'evaluation',
}

function MetricValue({
  label,
  metricName,
  value,
  metric,
  query,
  context = false,
}: {
  label: string
  metricName: ContainerMetricName
  value: ContainerMetricValue
  metric?: CanonicalMetric
  query?: MetricQuery
  context?: boolean
}) {
  const presentation = metricPresentation({
    canonicalName: metricName,
    scope: 'entity',
    fallbackTitle: metric?.display_name,
    fallbackMeaning: metric?.description,
  })
  const displayed = value.kind === 'single' ? formatMetricValue(value.value, value.unit) : undefined
  const text = value.kind === 'single'
    ? `${displayed?.value ?? value.value}${displayed?.unit ? ` ${displayed.unit}` : ''}${value.cumulative ? ' total' : ''}`
    : value.kind === 'multiple'
      ? `${value.count} series · details below`
      : context
        ? 'Not reported'
        : 'No current sample'

  return (
    <div className={styles.metricValue} data-kind={value.kind}>
      <span className={styles.metricLabel}>
        {label}
        <InfoTooltip
          canonicalName={metricName}
          idSuffix={label}
          sampleCount={value.kind === 'single' ? value.series.samples.length : value.kind === 'multiple' ? value.sampleCount : 0}
          sources={metric?.sources}
          meaning={presentation.meaning}
          observedSource={value.kind === 'single' ? value.sample.labels?.source : undefined}
          query={query ?? fallbackQuery}
          seriesLabels={presentation.seriesLabels}
        />
      </span>
      <strong>{text}</strong>
    </div>
  )
}

function ResourceRow({
  title,
  Icon,
  usedName,
  requestName,
  limitName,
  values,
  metrics,
  queries,
}: {
  title: string
  Icon: typeof Activity
  usedName: ContainerMetricName
  requestName: ContainerMetricName
  limitName: ContainerMetricName
  values: ContainerOverviewModel['cpu']
  metrics: Props['metrics']
  queries: Props['queries']
}) {
  return (
    <section className={styles.resourceRow} aria-label={`${title} resources`}>
      <header>
        <span><Icon size={15} aria-hidden="true" /> {title}</span>
        <p>Direct use with Kubernetes capacity context</p>
      </header>
      <div className={styles.resourceValues}>
        <MetricValue label="Used" metricName={usedName} value={values.used} metric={metrics[usedName]} query={queries[usedName]} />
        <MetricValue label="Request" metricName={requestName} value={values.request} metric={metrics[requestName]} query={queries[requestName]} context />
        <MetricValue label="Limit" metricName={limitName} value={values.limit} metric={metrics[limitName]} query={queries[limitName]} context />
      </div>
    </section>
  )
}

/** A summary-only Container projection; individual ambiguous series stay visible below. */
export function ContainerOverview({ overview, metrics, queries }: Props) {
  return (
    <section className={styles.card} aria-label="Container overview">
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}><ServerCog size={13} aria-hidden="true" /> Container overview</span>
          <h2>Resource use and current state</h2>
        </div>
        <div className={styles.status}>
          <span><Activity size={13} aria-hidden="true" /> Current status</span>
          <strong>{overview.status}</strong>
        </div>
      </header>

      <div className={styles.resources}>
        <ResourceRow
          title="CPU"
          Icon={Scale}
          usedName="container.cpu.usage"
          requestName="k8s.container.cpu.request"
          limitName="k8s.container.cpu.limit"
          values={overview.cpu}
          metrics={metrics}
          queries={queries}
        />
        <ResourceRow
          title="Memory"
          Icon={ServerCog}
          usedName="container.memory.usage"
          requestName="k8s.container.memory.request"
          limitName="k8s.container.memory.limit"
          values={overview.memory}
          metrics={metrics}
          queries={queries}
        />
      </div>

      <div className={styles.health}>
        <div>
          <span className={styles.healthLabel}><RotateCcw size={14} aria-hidden="true" /> Restart count</span>
          <MetricValue
            label="Observed cumulative restarts"
            metricName="k8s.container.restart.count"
            value={overview.restarts}
            metric={metrics['k8s.container.restart.count']}
            query={queries['k8s.container.restart.count']}
          />
        </div>
        <p><CircleHelp size={13} aria-hidden="true" /> Status is the backend-derived graph state. It combines Kubernetes readiness with the five-minute restart-rate check; this page does not reinterpret the numeric readiness metric.</p>
      </div>
    </section>
  )
}
