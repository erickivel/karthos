import { describe, expect, it } from 'vitest'
import { groupPodVolumeSeries } from './podVolumeModel'
import type { MetricSeries } from '@/types/metrics'

function series(metricName: string, labels: Record<string, string>, value: number): MetricSeries {
  return {
    metric_name: metricName,
    unit: 'bytes',
    category: 'capacity',
    description: 'fixture',
    samples: [{ metric_name: metricName, entity_id: 'pod-1', value, unit: 'bytes', timestamp: '2026-09-06T12:00:00Z', labels }],
  }
}

describe('groupPodVolumeSeries', () => {
  it('groups each metric family by the Pod-local volume name without aggregating values', () => {
    const groups = groupPodVolumeSeries({
      'k8s.volume.available': [series('k8s.volume.available', { volume_name: 'data', volume_type: 'pvc', persistentvolumeclaim_name: 'data-claim' }, 80)],
      'k8s.volume.capacity': [series('k8s.volume.capacity', { volume_name: 'data', volume_type: 'pvc', persistentvolumeclaim_name: 'data-claim' }, 100)],
      'k8s.volume.inodes.free': [series('k8s.volume.inodes.free', { volume_name: 'data' }, 40)],
      'k8s.volume.inodes.used': [series('k8s.volume.inodes.used', { volume_name: 'data' }, 60)],
      'k8s.volume.inodes': [series('k8s.volume.inodes', { volume_name: 'data' }, 100)],
    })

    expect(groups).toHaveLength(1)
    expect(groups[0]).toMatchObject({
      name: 'data',
      volumeType: 'pvc',
      persistentVolumeClaimName: 'data-claim',
      available: { value: 80 },
      capacity: { value: 100 },
      inodesFree: { value: 40 },
      inodesUsed: { value: 60 },
      inodes: { value: 100 },
    })
  })

  it('keeps each unlabeled series as its own truthful fallback row', () => {
    const groups = groupPodVolumeSeries({
      'k8s.volume.available': [
        series('k8s.volume.available', { device: '/dev/a' }, 10),
        series('k8s.volume.available', { device: '/dev/b' }, 20),
      ],
    })

    expect(groups).toHaveLength(2)
    expect(groups.map((group) => group.name)).toEqual(['Unnamed volume', 'Unnamed volume'])
    expect(groups.map((group) => group.available?.value)).toEqual([10, 20])
  })

  it('uses the newest timestamp rather than the input order for a mount series', () => {
    const available = series('k8s.volume.available', { volume_name: 'data' }, 80)
    available.samples.unshift({
      metric_name: 'k8s.volume.available',
      entity_id: 'pod-1',
      value: 100,
      unit: 'bytes',
      timestamp: '2026-09-06T12:05:00Z',
      labels: { volume_name: 'data' },
    })

    const [group] = groupPodVolumeSeries({ 'k8s.volume.available': [available] })
    expect(group?.available?.value).toBe(100)
  })
})
