import { describe, expect, it } from 'vitest'
import type { MetricSample } from '@/types/metrics'
import { seriesIdentity } from './rawSampleIdentity'

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
      sample('k8s.volume.capacity', { volume_name: 'config-volume', volume_type: 'emptyDir' }),
      sample('k8s.volume.capacity', { volume_name: 'cache-volume', volume_type: 'emptyDir' }),
    ]
    expect(seriesIdentity(rows, rows[0]!)).toEqual({
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
    expect(seriesIdentity(rows, rows[0]!).summary).toBe('cpu=cpu0 · state=idle')
  })

  it('keeps enum value labels separate from series dimensions', () => {
    const row = { ...sample('k8s.pod.phase', {}), value_label: 'running' }
    expect(seriesIdentity([row], row).summary).toBe('')
  })

  it('distinguishes a missing dimension from a populated one', () => {
    const rows = [
      sample('system.filesystem.usage', { mountpoint: '/' }),
      sample('system.filesystem.usage', { mountpoint: '/', state: 'used' }),
    ]
    expect(seriesIdentity(rows, rows[0]!).summary).toContain('state=(unset)')
  })
})
