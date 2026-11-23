import { describe, expect, it } from 'vitest'
import type { MetricSample } from '@/types/metrics'
import { seriesIdentities } from './rawSampleIdentity'

const sample = (metric_name: string, labels: Record<string, string>): MetricSample => ({
  metric_name,
  entity_id: 'pod-1',
  value: 1,
  timestamp: '2026-10-11T12:00:00Z',
  unit: 'bytes',
  labels: { source: 'kubelet', ...labels },
})

describe('raw sample identity', () => {
  it('shows the dimension that distinguishes volume rows and exposes all labels', () => {
    const rows = [
      sample('k8s.pod.volume.capacity', { volume_name: 'config-volume', volume_type: 'emptyDir' }),
      sample('k8s.pod.volume.capacity', { volume_name: 'cache-volume', volume_type: 'emptyDir' }),
    ]
    expect(seriesIdentities(rows)[0]).toEqual({
      summary: 'volume_name=config-volume',
      detail: 'source=kubelet\nvolume_name=config-volume\nvolume_type=emptyDir',
    })
  })

  it('keeps independent host core and state dimensions in the visible identity', () => {
    const rows = [
      sample('system.cpu.utilization', { cpu: 'cpu0', state: 'idle' }),
      sample('system.cpu.utilization', { cpu: 'cpu1', state: 'idle' }),
      sample('system.cpu.utilization', { cpu: 'cpu0', state: 'user' }),
    ]
    expect(seriesIdentities(rows)[0]?.summary).toBe('cpu=cpu0 · state=idle')
  })

  it('keeps enum value labels separate from series dimensions', () => {
    const row = { ...sample('k8s.pod.status.phase', {}), value_label: 'running' }
    expect(seriesIdentities([row])[0]?.summary).toBe('')
  })

  it('distinguishes a missing dimension from a populated one', () => {
    const rows = [
      sample('system.filesystem.usage', { mountpoint: '/' }),
      sample('system.filesystem.usage', { mountpoint: '/', state: 'used' }),
    ]
    expect(seriesIdentities(rows)[0]?.summary).toContain('state=(unset)')
  })

  it('groups independently by metric, scope, and source while preserving row order', () => {
    const samePod = sample('k8s.pod.volume.capacity', { volume_name: 'config-volume' })
    const otherVolume = sample('k8s.pod.volume.capacity', { volume_name: 'cache-volume' })
    const otherPod = { ...sample('k8s.pod.volume.capacity', { volume_name: 'logs-volume' }), entity_id: 'pod-2' }
    const otherSource = sample('k8s.pod.volume.capacity', { source: 'another-source', volume_name: 'logs-volume' })
    const otherMetric = sample('k8s.pod.volume.available', { volume_name: 'logs-volume' })
    const rows = [samePod, otherVolume, otherPod, otherSource, otherMetric, samePod]

    expect(seriesIdentities(rows).map(({ summary }) => summary)).toEqual([
      'volume_name=config-volume',
      'volume_name=cache-volume',
      'volume_name=logs-volume',
      'volume_name=logs-volume',
      'volume_name=logs-volume',
      'volume_name=config-volume',
    ])
  })

  it('keeps edge groups separate even when endpoint IDs overlap with entity IDs', () => {
    const edge = { source_id: 'pod-1', target_id: 'pod-2' }
    const rows = [
      { ...sample('servicegraph.request.count', { status: '200' }), edge },
      { ...sample('servicegraph.request.count', { status: '500' }), edge },
      { ...sample('servicegraph.request.count', { status: '404' }), edge: { source_id: 'pod-1', target_id: 'pod-3' } },
      sample('servicegraph.request.count', { status: '201' }),
    ]

    expect(seriesIdentities(rows).map(({ summary }) => summary)).toEqual([
      'status=200', 'status=500', 'status=404', 'status=201',
    ])
  })

  it('does not repeatedly scan peer labels for repeated timestamps', () => {
    let enumerations = 0
    const labels = new Proxy({ source: 'kubelet', volume_name: 'config-volume' }, {
      ownKeys(target) {
        enumerations += 1
        return Reflect.ownKeys(target)
      },
    })
    const rows = Array.from({ length: 500 }, (_, i) => ({
      ...sample('k8s.pod.volume.capacity', {}), labels, timestamp: new Date(i * 1000).toISOString(),
    }))

    const identities = seriesIdentities(rows)
    expect(identities).toHaveLength(rows.length)
    expect(identities[499]?.summary).toBe('volume_name=config-volume')
    expect(enumerations).toBeLessThan(2_500)
  })
})
