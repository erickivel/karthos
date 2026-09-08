import { describe, expect, it } from 'vitest'
import { latestSample } from './metricSeries'
import type { MetricSample } from '@/types/metrics'

function sample(timestamp: string, value: number): MetricSample {
  return {
    metric_name: 'k8s.node.cpu.usage',
    entity_id: 'node:demo:worker-a',
    value,
    timestamp,
    unit: 'millicores',
  }
}

describe('latestSample', () => {
  it('selects the newest producer timestamp regardless of response order', () => {
    expect(latestSample([
      sample('2026-09-07T12:00:30Z', 300),
      sample('2026-09-07T12:01:00Z', 450),
      sample('2026-09-07T12:00:45Z', 375),
    ])).toMatchObject({
      timestamp: '2026-09-07T12:01:00Z',
      value: 450,
    })
  })
})
