import type { MetricSeries } from '@/types/metrics'

export interface DerivedCpuSample {
  timestamp: string
  value: number
}

export interface DerivedNodeCpuUtilization {
  samples: DerivedCpuSample[]
  current?: DerivedCpuSample
  observedCoreCount: number
}

/**
 * Derives an optional, presentation-only node utilization value from host CPU
 * idle series. It deliberately rejects partial or duplicated core input rather
 * than inventing a percentage from an incomplete view of the node.
 */
export function deriveNodeCpuUtilization(series: MetricSeries[]): DerivedNodeCpuUtilization {
  const hostSeries = series.filter((entry) => entry.samples[0]?.labels?.source === 'host-os')
  const cores = new Set<string>()
  const idleValuesByTimestamp = new Map<string, Map<string, number[]>>()
  let latestObservedTimestamp: string | undefined

  for (const entry of hostSeries) {
    const labels = entry.samples[0]?.labels
    const cpu = labels?.cpu
    if (!cpu) continue
    cores.add(cpu)

    for (const sample of entry.samples) {
      if (!latestObservedTimestamp || sample.timestamp > latestObservedTimestamp) {
        latestObservedTimestamp = sample.timestamp
      }
      if (sample.labels?.state !== 'idle') continue
      const byCore = idleValuesByTimestamp.get(sample.timestamp) ?? new Map<string, number[]>()
      const values = byCore.get(cpu) ?? []
      values.push(sample.value)
      byCore.set(cpu, values)
      idleValuesByTimestamp.set(sample.timestamp, byCore)
    }
  }

  const samples = [...idleValuesByTimestamp.entries()]
    .flatMap(([timestamp, byCore]) => {
      if (cores.size === 0 || byCore.size !== cores.size) return []
      const idleValues: number[] = []
      for (const cpu of cores) {
        const values = byCore.get(cpu)
        const value = values?.[0]
        if (!values || values.length !== 1 || value === undefined || !Number.isFinite(value) || value < 0 || value > 1) return []
        idleValues.push(value)
      }
      const idleAverage = idleValues.reduce((total, value) => total + value, 0) / idleValues.length
      return [{ timestamp, value: Math.round(10000 * (1 - idleAverage)) / 100 }]
    })
    .sort((left, right) => left.timestamp.localeCompare(right.timestamp))

  return {
    samples,
    current: samples.find((sample) => sample.timestamp === latestObservedTimestamp),
    observedCoreCount: cores.size,
  }
}
