import { describe, expect, it } from 'vitest'
import { deriveNodeCpuUtilization } from './nodeCpuDerivation'

function cpuSeries(cpu: string, state: string, samples: Array<[string, number]>) {
  return {
    metric_name: 'system.cpu.utilization',
    unit: 'percent',
    category: 'resource' as const,
    description: '',
    samples: samples.map(([timestamp, value]) => ({
      metric_name: 'system.cpu.utilization',
      entity_id: 'node-1',
      value,
      timestamp,
      unit: 'percent',
      labels: { cpu, state, source: 'host-os' },
    })),
  }
}

describe('deriveNodeCpuUtilization', () => {
  it('derives utilization from one idle sample for every observed CPU core', () => {
    const result = deriveNodeCpuUtilization([
      cpuSeries('cpu0', 'idle', [['2026-09-06T12:00:00Z', 90]]),
      cpuSeries('cpu1', 'idle', [['2026-09-06T12:00:00Z', 80]]),
      cpuSeries('cpu0', 'user', [['2026-09-06T12:00:00Z', 5]]),
      cpuSeries('cpu1', 'user', [['2026-09-06T12:00:00Z', 10]]),
    ])

    expect(result.observedCoreCount).toBe(2)
    expect(result.samples).toEqual([{ timestamp: '2026-09-06T12:00:00Z', value: 15 }])
    expect(result.current).toEqual({ timestamp: '2026-09-06T12:00:00Z', value: 15 })
  })

  it.each([
    ['a missing core', [
      cpuSeries('cpu0', 'idle', [['2026-09-06T12:00:00Z', 90]]),
      cpuSeries('cpu1', 'user', [['2026-09-06T12:00:00Z', 10]]),
    ]],
    ['a duplicate core sample', [
      cpuSeries('cpu0', 'idle', [['2026-09-06T12:00:00Z', 90]]),
      cpuSeries('cpu0', 'idle', [['2026-09-06T12:00:00Z', 88]]),
      cpuSeries('cpu1', 'idle', [['2026-09-06T12:00:00Z', 80]]),
    ]],
  ])('suppresses a calculation when there is %s', (_reason, series) => {
    expect(deriveNodeCpuUtilization(series).current).toBeUndefined()
  })

  it('does not present an earlier complete value as current when an idle core is stale', () => {
    const result = deriveNodeCpuUtilization([
      cpuSeries('cpu0', 'idle', [
        ['2026-09-06T12:00:00Z', 90],
        ['2026-09-06T12:00:15Z', 90],
      ]),
      cpuSeries('cpu1', 'idle', [['2026-09-06T12:00:00Z', 80]]),
      cpuSeries('cpu1', 'user', [['2026-09-06T12:00:15Z', 10]]),
    ])

    expect(result.samples).toEqual([{ timestamp: '2026-09-06T12:00:00Z', value: 15 }])
    expect(result.current).toBeUndefined()
  })
})
