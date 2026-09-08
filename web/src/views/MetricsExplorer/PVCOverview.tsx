import { ChevronDown, CircleHelp, Database, HardDrive, Layers3 } from 'lucide-react'
import type { CanonicalMetric, MetricQuery } from '@/types/metrics'
import { formatMetricValue } from '@/utils/formatters'
import { InfoTooltip } from './InfoTooltip'
import { pvcMetricPresentation, type PVCMetricName, type PVCMetricValue, type PVCOverview as PVCOverviewModel, type PVCStorageSnapshot } from './pvcOverviewModel'
import styles from './PVCOverview.module.css'

interface Props {
  overview: PVCOverviewModel
  metrics: Partial<Record<PVCMetricName, CanonicalMetric>>
  queries: Partial<Record<PVCMetricName, MetricQuery>>
}

const fallbackQuery: MetricQuery = {
  aggregation: 'latest',
  from: '',
  to: '',
  step: '15s',
  timestamp_semantics: 'evaluation',
}

const FIELD_BY_METRIC: Record<PVCMetricName, keyof Pick<PVCStorageSnapshot, 'available' | 'capacity' | 'inodesFree' | 'inodesUsed' | 'inodes'>> = {
  'k8s.volume.available': 'available',
  'k8s.volume.capacity': 'capacity',
  'k8s.volume.inodes.free': 'inodesFree',
  'k8s.volume.inodes.used': 'inodesUsed',
  'k8s.volume.inodes': 'inodes',
}

function humanLabel(label: string) {
  return ({
    volume_name: 'Volume',
    volume_type: 'Volume type',
    persistentvolumeclaim_name: 'Claim',
    source: 'Source',
  } as Record<string, string>)[label] ?? label.replace(/_/g, ' ')
}

function seriesLabels(snapshot: PVCStorageSnapshot) {
  return Object.entries(snapshot.labels)
    .sort(([left], [right]) => left.localeCompare(right))
    .map(([label, value]) => ({ label: humanLabel(label), value }))
}

function valueText(value?: PVCMetricValue) {
  if (!value) return 'No current sample'
  const formatted = formatMetricValue(value.value, value.unit)
  return `${formatted.value}${formatted.unit ? ` ${formatted.unit}` : ''}`
}

function Snapshot({ snapshot, metrics, queries }: {
  snapshot: PVCStorageSnapshot
  metrics: Props['metrics']
  queries: Props['queries']
}) {
  const fact = (metricName: PVCMetricName) => {
    const field = FIELD_BY_METRIC[metricName]
    const value = snapshot[field]
    const metric = metrics[metricName]
    const presentation = pvcMetricPresentation(metricName)
    return (
      <div className={styles.metricFact} data-empty={value ? undefined : 'true'}>
        <span>
          {presentation.title}
          <InfoTooltip
            canonicalName={metricName}
            idSuffix={`${snapshot.key}-${metricName}`}
            sampleCount={value?.series.samples.length ?? 0}
            sources={metric?.sources}
            meaning={presentation.meaning}
            observedSource={value?.sample.labels?.source}
            query={queries[metricName] ?? fallbackQuery}
            seriesLabels={seriesLabels(snapshot)}
          />
        </span>
        <strong>{valueText(value)}</strong>
      </div>
    )
  }

  return (
    <section className={styles.snapshot} aria-label={`PVC storage series ${snapshot.identity}`}>
      <header className={styles.snapshotHeader}>
        <span><Database size={14} aria-hidden="true" /> {snapshot.identity}</span>
        <small>One label-distinct series</small>
      </header>
      <div className={styles.capacity}>{fact('k8s.volume.available')}{fact('k8s.volume.capacity')}</div>
      <details className={styles.inodes}>
        <summary>
          <span><Layers3 size={14} aria-hidden="true" /> Inode details</span>
          <span className={styles.inodePreview}>{valueText(snapshot.inodesFree)} free · {valueText(snapshot.inodesUsed)} used</span>
          <ChevronDown size={16} aria-hidden="true" />
        </summary>
        <div className={styles.inodeValues}>
          {fact('k8s.volume.inodes.free')}
          {fact('k8s.volume.inodes.used')}
          {fact('k8s.volume.inodes')}
        </div>
      </details>
    </section>
  )
}

/** A PVC-specific, no-aggregation capacity presentation over complete cursor results. */
export function PVCOverview({ overview, metrics, queries }: Props) {
  if (overview.kind === 'empty') {
    return (
      <section className={`${styles.card} ${styles.empty}`} aria-label="PVC storage">
        <header className={styles.header}>
          <div>
            <span className={styles.eyebrow}><HardDrive size={13} aria-hidden="true" /> PVC storage</span>
            <h2>No current volume sample</h2>
          </div>
        </header>
        <p><CircleHelp size={14} aria-hidden="true" /> No current Kubelet volume sample for this claim. A structural graph relationship proves that the claim is referenced; it is not filesystem telemetry.</p>
      </section>
    )
  }

  const multiple = overview.kind === 'multiple'
  return (
    <section className={styles.card} aria-label="PVC storage">
      <header className={styles.header}>
        <div>
          <span className={styles.eyebrow}><HardDrive size={13} aria-hidden="true" /> PVC storage</span>
          <h2>{multiple ? `${overview.seriesCount} current series` : 'Storage snapshot'}</h2>
        </div>
        <p className={styles.headerNote}>{multiple ? 'Values are not combined across label-distinct series.' : 'Direct Kubelet capacity facts.'}</p>
      </header>
      {multiple ? (
        <details className={styles.seriesDetails}>
          <summary><span>Inspect {overview.seriesCount} series</span><ChevronDown size={16} aria-hidden="true" /></summary>
          <div className={styles.seriesList}>
            {overview.snapshots.map((snapshot) => <Snapshot key={snapshot.key} snapshot={snapshot} metrics={metrics} queries={queries} />)}
          </div>
        </details>
      ) : <Snapshot snapshot={overview.snapshots[0]!} metrics={metrics} queries={queries} />}
    </section>
  )
}
