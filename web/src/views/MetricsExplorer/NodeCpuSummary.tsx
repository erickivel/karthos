import type { MetricSeries, SourceRef } from '@/types/metrics'
import { formatValue } from '@/utils/formatters'
import { latestSample } from '@/utils/metricSeries'
import type { DerivedNodeCpuUtilization } from './nodeCpuDerivation'
import styles from './NodeCpuSummary.module.css'

interface Props {
  directSeries?: MetricSeries
  derived: DerivedNodeCpuUtilization
  sources?: SourceRef[]
}

export function NodeCpuSummary({ directSeries, derived, sources }: Props) {
  const directSample = latestSample(directSeries?.samples)
  const observedSource = directSample?.labels?.source
  const directValue = directSample ? `${Math.round(directSample.value)} mCPU used` : 'No current kubelet CPU sample'
  const sortedSources = [...(sources ?? [])].sort((left, right) => left.priority - right.priority)

  return (
    <section className={styles.card} aria-label="Node CPU summary">
      <div className={styles.header}>
        <div>
          <span className={styles.eyebrow}>Node resource</span>
          <h2>CPU</h2>
        </div>
        {observedSource && <span className={styles.source}>source: {observedSource}</span>}
      </div>
      <div className={styles.values}>
        <div className={styles.primaryValue}>{directValue}</div>
        {derived.current ? (
          <div className={styles.derivedValue}>{formatValue(derived.current.value)}% utilized</div>
        ) : (
          <div className={styles.unavailable}>Utilization unavailable: complete current idle data is required</div>
        )}
      </div>
      <details className={styles.details}>
        <summary>How this is calculated</summary>
        <p><code>k8s.node.cpu.usage</code> is the direct kubelet CPU measurement, shown in mCPU.</p>
        <p><code>system.cpu.utilization</code> adds context only: <code>100 − average(idle)</code> across {derived.observedCoreCount || 'all observed'} host CPU cores at the newest evaluation point.</p>
        {sortedSources.length > 0 && (
          <p>Declared source priority: {sortedSources.map((source) => `${source.id} (${source.priority})`).join(', ')}.</p>
        )}
      </details>
    </section>
  )
}
