import type { MetricSample, MetricSeries } from '@/types/metrics'

export const PVC_METRIC_NAMES = [
  'k8s.pod.volume.available',
  'k8s.pod.volume.capacity',
  'k8s.pod.volume.inodes.free',
  'k8s.pod.volume.inodes.used',
  'k8s.pod.volume.inodes',
] as const

export type PVCMetricName = (typeof PVC_METRIC_NAMES)[number]
export type PVCSeriesByMetric = Partial<Record<PVCMetricName, MetricSeries[]>>

export interface PVCMetricValue {
  value: number
  unit: string
  sample: MetricSample
  series: MetricSeries
}

export interface PVCStorageSnapshot {
  key: string
  identity: string
  labels: Record<string, string>
  available?: PVCMetricValue
  capacity?: PVCMetricValue
  inodesFree?: PVCMetricValue
  inodesUsed?: PVCMetricValue
  inodes?: PVCMetricValue
}

export interface PVCOverview {
  kind: 'empty' | 'single' | 'multiple'
  seriesCount: number
  snapshots: PVCStorageSnapshot[]
}

export interface PVCMetricPresentation {
  title: string
  meaning: string
}

const FIELD_BY_METRIC: Record<PVCMetricName, keyof Omit<PVCStorageSnapshot, 'key' | 'identity' | 'labels'>> = {
  'k8s.pod.volume.available': 'available',
  'k8s.pod.volume.capacity': 'capacity',
  'k8s.pod.volume.inodes.free': 'inodesFree',
  'k8s.pod.volume.inodes.used': 'inodesUsed',
  'k8s.pod.volume.inodes': 'inodes',
}

const PRESENTATION: Record<PVCMetricName, PVCMetricPresentation> = {
  'k8s.pod.volume.available': {
    title: 'Available storage',
    meaning: 'Storage currently available on this PersistentVolumeClaim, reported by kubelet.',
  },
  'k8s.pod.volume.capacity': {
    title: 'Storage capacity',
    meaning: 'Total storage capacity reported for this PersistentVolumeClaim by kubelet.',
  },
  'k8s.pod.volume.inodes.free': {
    title: 'Free inodes',
    meaning: 'Filesystem inodes still available on this PersistentVolumeClaim.',
  },
  'k8s.pod.volume.inodes.used': {
    title: 'Used inodes',
    meaning: 'Filesystem inodes currently used on this PersistentVolumeClaim.',
  },
  'k8s.pod.volume.inodes': {
    title: 'Total inodes',
    meaning: 'Total filesystem inodes reported for this PersistentVolumeClaim.',
  },
}

function latestSample(series: MetricSeries): MetricSample | undefined {
  const sorted = [...series.samples].sort((left, right) => (
    new Date(left.timestamp).getTime() - new Date(right.timestamp).getTime()
  ))
  return sorted[sorted.length - 1]
}

function keyFor(labels: Record<string, string>, metricName: PVCMetricName, index: number): string {
  const entries = Object.entries(labels).sort(([left], [right]) => left.localeCompare(right))
  return entries.length > 0 ? JSON.stringify(entries) : `${metricName}:${index}:unlabeled`
}

function identityFor(labels: Record<string, string>): string {
  const values = [labels.volume_name, labels.volume_type, labels.source].filter(Boolean)
  return values.length > 0 ? values.join(' · ') : 'Unlabelled volume series'
}

export function pvcMetricPresentation(metricName: PVCMetricName): PVCMetricPresentation {
  return PRESENTATION[metricName]
}

/**
 * Builds a PVC projection without aggregating, selecting, or combining
 * label-distinct series. A primary snapshot exists only when all observed
 * metric families identify one shared dimensional series.
 */
export function buildPVCOverview(seriesByMetric: PVCSeriesByMetric): PVCOverview {
  const snapshots = new Map<string, PVCStorageSnapshot>()

  for (const metricName of PVC_METRIC_NAMES) {
    for (const [index, series] of (seriesByMetric[metricName] ?? []).entries()) {
      const sample = latestSample(series)
      if (!sample) continue
      const labels = sample.labels ?? {}
      const key = keyFor(labels, metricName, index)
      const snapshot = snapshots.get(key) ?? {
        key,
        identity: identityFor(labels),
        labels: { ...labels },
      }
      snapshot[FIELD_BY_METRIC[metricName]] = {
        value: sample.value,
        unit: series.unit,
        sample,
        series,
      }
      snapshots.set(key, snapshot)
    }
  }

  const values = [...snapshots.values()].sort((left, right) => left.identity.localeCompare(right.identity) || left.key.localeCompare(right.key))
  return {
    kind: values.length === 0 ? 'empty' : values.length === 1 ? 'single' : 'multiple',
    seriesCount: values.length,
    snapshots: values,
  }
}
