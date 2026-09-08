import { Activity, CircleHelp, RadioTower } from 'lucide-react'
import type { CanonicalMetric, MetricQuery, MetricSeries } from '@/types/metrics'
import { InfoTooltip } from './InfoTooltip'
import { podEnumLabel } from './podHealthModel'
import { latestSample } from '@/utils/metricSeries'
import styles from './PodHealthSummary.module.css'

interface Props {
  phaseMetric?: CanonicalMetric
  reasonMetric?: CanonicalMetric
  phase?: MetricSeries
  reason?: MetricSeries
  query?: MetricQuery
}

function enumRow(label: string, value: string, source?: string, timestamp?: string, secondary = false) {
  return (
    <div className={`${styles.row} ${secondary ? styles.secondary : ''}`}>
      <div>
        <span className={styles.label}>{label}</span>
        <strong>{value}</strong>
      </div>
      <span className={styles.meta}>
        {source && <><RadioTower size={12} aria-hidden="true" /> {source}</>}
        {timestamp && <time dateTime={timestamp}>{new Date(timestamp).toLocaleTimeString()}</time>}
      </span>
    </div>
  )
}

/** A textual view of Pod enum samples. Numeric enum values remain available in raw samples. */
export function PodHealthSummary({ phaseMetric, reasonMetric, phase, reason, query }: Props) {
  const phaseSample = latestSample(phase?.samples)
  const reasonSample = latestSample(reason?.samples)
  const phaseValue = podEnumLabel(phase)
  const reasonValue = podEnumLabel(reason)
  const phaseSource = phaseSample?.labels?.source
  const reasonSource = reasonSample?.labels?.source
  const fallbackQuery: MetricQuery = {
    aggregation: 'latest',
    from: '',
    to: '',
    step: '15s',
    timestamp_semantics: 'evaluation',
  }

  return (
    <section className={styles.card} aria-label="Pod health">
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}><Activity size={13} aria-hidden="true" /> Pod health</span>
          <h2>Current Kubernetes status</h2>
        </div>
        <InfoTooltip
          canonicalName={phaseMetric?.name ?? 'k8s.pod.phase'}
          sampleCount={(phase?.samples.length ?? 0) + (reason?.samples.length ?? 0)}
          sources={phaseMetric?.sources}
          meaning="The current Kubernetes Pod phase and separately reported diagnostic reason. These labels do not replace graph health."
          observedSource={phaseSource}
          query={query ?? fallbackQuery}
          seriesLabels={[]}
        />
      </header>
      <div className={styles.rows}>
        {enumRow('Phase', phaseValue, phaseSource, phaseSample?.timestamp)}
        {enumRow('Reported reason', reasonValue, reasonSource, reasonSample?.timestamp, true)}
      </div>
      <p className={styles.note}><CircleHelp size={13} aria-hidden="true" /> “Unknown” is a neutral receiver-reported value, not a healthy or unhealthy conclusion.</p>
      {(phaseMetric || reasonMetric) && (
        <details className={styles.details}>
          <summary>Metric details</summary>
          <p><code>{phaseMetric?.name ?? 'k8s.pod.phase'}</code> and <code>{reasonMetric?.name ?? 'k8s.pod.status_reason'}</code> remain numeric canonical samples; the API adds their readable labels.</p>
        </details>
      )}
    </section>
  )
}
