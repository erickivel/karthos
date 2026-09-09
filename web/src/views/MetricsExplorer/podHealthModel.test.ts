import { describe, expect, it } from 'vitest'
import { podEnumLabel } from './podHealthModel'
import type { MetricSeries } from '@/types/metrics'

function series(valueLabel?: string): MetricSeries {
  return {
    metric_name: 'k8s.pod.phase',
    unit: '1',
    category: 'health',
    description: 'fixture',
    samples: [{
      metric_name: 'k8s.pod.phase', entity_id: 'pod-1', value: 2, unit: '1',
      timestamp: '2026-09-06T12:00:00Z', labels: {}, value_label: valueLabel,
    }],
  }
}

describe('podEnumLabel', () => {
  it('renders the API-provided phase label without translating the numeric sample in the UI', () => {
    expect(podEnumLabel(series('running'))).toBe('Running')
  })

  it('keeps the receiver-reported unknown value neutral', () => {
    expect(podEnumLabel(series('unknown'))).toBe('Unknown')
  })

  it('distinguishes a missing API label from the receiver-reported unknown value', () => {
    expect(podEnumLabel(series())).toBe('No readable API label')
    expect(podEnumLabel()).toBe('No current sample')
  })
})
