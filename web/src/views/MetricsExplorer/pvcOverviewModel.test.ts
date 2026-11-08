import { describe, expect, it } from 'vitest'
import { buildPVCOverview, PVC_METRIC_NAMES, pvcMetricPresentation } from './pvcOverviewModel'
import type { MetricSeries } from '@/types/metrics'

function series(
  metricName: string,
  value: number,
  labels: Record<string, string> = {},
  timestamp = '2026-09-07T01:28:00Z',
): MetricSeries {
  return {
    metric_name: metricName,
    unit: metricName.includes('inodes') ? 'count' : 'bytes',
    category: 'capacity',
    description: 'fixture',
    samples: [{ metric_name: metricName, entity_id: 'pvc-1', value, unit: metricName.includes('inodes') ? 'count' : 'bytes', timestamp, labels }],
  }
}

const dataLabels = {
  persistentvolumeclaim_name: 'data-karthos-postgresql-0',
  volume_name: 'data',
  volume_type: 'persistentVolumeClaim',
  source: 'kubelet',
}

describe('PVC overview projection', () => {
  it('declares the five supported PVC volume metrics with plain-language titles', () => {
    expect(PVC_METRIC_NAMES).toEqual([
      'k8s.pod.volume.available',
      'k8s.pod.volume.capacity',
      'k8s.pod.volume.inodes.free',
      'k8s.pod.volume.inodes.used',
      'k8s.pod.volume.inodes',
    ])
    expect(pvcMetricPresentation('k8s.pod.volume.available')).toMatchObject({ title: 'Available storage' })
    expect(pvcMetricPresentation('k8s.pod.volume.capacity')).toMatchObject({ title: 'Storage capacity' })
    expect(pvcMetricPresentation('k8s.pod.volume.inodes.free')).toMatchObject({ title: 'Free inodes' })
    expect(pvcMetricPresentation('k8s.pod.volume.inodes.used')).toMatchObject({ title: 'Used inodes' })
    expect(pvcMetricPresentation('k8s.pod.volume.inodes')).toMatchObject({ title: 'Total inodes' })
  })

  it('builds one direct storage snapshot without deriving used space or a percentage', () => {
    const overview = buildPVCOverview({
      'k8s.pod.volume.available': [series('k8s.pod.volume.available', 180_435_808_256, dataLabels)],
      'k8s.pod.volume.capacity': [series('k8s.pod.volume.capacity', 231_922_479_104, dataLabels)],
      'k8s.pod.volume.inodes.free': [series('k8s.pod.volume.inodes.free', 14_047_173, dataLabels)],
      'k8s.pod.volume.inodes.used': [series('k8s.pod.volume.inodes.used', 411_707, dataLabels)],
      'k8s.pod.volume.inodes': [series('k8s.pod.volume.inodes', 14_458_880, dataLabels)],
    })

    expect(overview).toMatchObject({ kind: 'single', seriesCount: 1 })
    expect(overview.snapshots[0]).toMatchObject({
      identity: 'data · persistentVolumeClaim · kubelet',
      available: { value: 180_435_808_256 },
      capacity: { value: 231_922_479_104 },
      inodesFree: { value: 14_047_173 },
      inodesUsed: { value: 411_707 },
      inodes: { value: 14_458_880 },
    })
    expect(overview.snapshots[0]).not.toHaveProperty('used')
    expect(overview.snapshots[0]).not.toHaveProperty('utilization')
  })

  it('keeps label-distinct PVC series as separate drill-down snapshots', () => {
    const overview = buildPVCOverview({
      'k8s.pod.volume.available': [
        series('k8s.pod.volume.available', 80, { ...dataLabels, volume_name: 'data-a' }),
        series('k8s.pod.volume.available', 40, { ...dataLabels, volume_name: 'data-b' }),
      ],
      'k8s.pod.volume.capacity': [
        series('k8s.pod.volume.capacity', 100, { ...dataLabels, volume_name: 'data-a' }),
        series('k8s.pod.volume.capacity', 50, { ...dataLabels, volume_name: 'data-b' }),
      ],
    })

    expect(overview).toMatchObject({ kind: 'multiple', seriesCount: 2 })
    expect(overview.snapshots.map((snapshot) => snapshot.identity)).toEqual([
      'data-a · persistentVolumeClaim · kubelet',
      'data-b · persistentVolumeClaim · kubelet',
    ])
    expect(overview.snapshots.map((snapshot) => snapshot.available?.value)).toEqual([80, 40])
  })

  it('returns an explicit no-samples state for a structural-only claim', () => {
    expect(buildPVCOverview({})).toEqual({ kind: 'empty', seriesCount: 0, snapshots: [] })
  })
})
