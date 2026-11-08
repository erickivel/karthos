import type { MetricSeries } from '@/types/metrics'

export const POD_VOLUME_METRICS = [
  'k8s.pod.volume.available',
  'k8s.pod.volume.capacity',
  'k8s.pod.volume.inodes.free',
  'k8s.pod.volume.inodes.used',
  'k8s.pod.volume.inodes',
] as const

type PodVolumeMetricName = (typeof POD_VOLUME_METRICS)[number]
type VolumeSeriesByMetric = Partial<Record<PodVolumeMetricName, MetricSeries[]>>

export interface PodVolumeSample {
  value: number
  unit: string
  timestamp: string
  labels: Record<string, string>
}

export interface PodVolumeGroup {
  key: string
  name: string
  volumeType?: string
  persistentVolumeClaimName?: string
  available?: PodVolumeSample
  capacity?: PodVolumeSample
  inodesFree?: PodVolumeSample
  inodesUsed?: PodVolumeSample
  inodes?: PodVolumeSample
}

const FIELD_BY_METRIC: Record<PodVolumeMetricName, keyof Omit<PodVolumeGroup, 'key' | 'name' | 'volumeType' | 'persistentVolumeClaimName'>> = {
  'k8s.pod.volume.available': 'available',
  'k8s.pod.volume.capacity': 'capacity',
  'k8s.pod.volume.inodes.free': 'inodesFree',
  'k8s.pod.volume.inodes.used': 'inodesUsed',
  'k8s.pod.volume.inodes': 'inodes',
}

function latestSample(series: MetricSeries): PodVolumeSample | undefined {
  const sortedSamples = [...series.samples].sort((left, right) => (
    new Date(left.timestamp).getTime() - new Date(right.timestamp).getTime()
  ))
  const sample = sortedSamples[sortedSamples.length - 1]
  if (!sample) return undefined
  return {
    value: sample.value,
    unit: series.unit,
    timestamp: sample.timestamp,
    labels: sample.labels ?? {},
  }
}

function fallbackIdentity(labels: Record<string, string>, metricName: string, index: number): string {
  const sorted = Object.entries(labels).sort(([left], [right]) => left.localeCompare(right))
  return `${metricName}:${index}:${JSON.stringify(sorted)}`
}

/**
 * Collects the latest value from each returned metric family by mount. It
 * intentionally does not sum, divide, or infer cross-volume capacity: mounts
 * can describe unrelated backing filesystems.
 */
export function groupPodVolumeSeries(seriesByMetric: VolumeSeriesByMetric): PodVolumeGroup[] {
  const groups = new Map<string, PodVolumeGroup>()

  for (const metricName of POD_VOLUME_METRICS) {
    for (const [index, series] of (seriesByMetric[metricName] ?? []).entries()) {
      const sample = latestSample(series)
      if (!sample) continue
      const volumeName = sample.labels.volume_name
      const key = volumeName ? `named:${volumeName}` : `unnamed:${fallbackIdentity(sample.labels, metricName, index)}`
      const group = groups.get(key) ?? {
        key,
        name: volumeName || 'Unnamed volume',
        volumeType: sample.labels.volume_type,
        persistentVolumeClaimName: sample.labels.persistentvolumeclaim_name,
      }
      const field = FIELD_BY_METRIC[metricName]
      group[field] = sample
      groups.set(key, group)
    }
  }

  return [...groups.values()].sort((left, right) => left.name.localeCompare(right.name) || left.key.localeCompare(right.key))
}
