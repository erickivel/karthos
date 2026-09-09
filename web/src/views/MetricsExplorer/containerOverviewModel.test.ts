import { describe, expect, it } from 'vitest'
import { buildContainerOverview, CONTAINER_METRIC_NAMES } from './containerOverviewModel'
import type { MetricSeries } from '@/types/metrics'

function series(metricName: string, value: number, unit: string, timestamp = '2026-09-06T12:00:00Z'): MetricSeries {
  return {
    metric_name: metricName,
    unit,
    category: 'resource',
    description: 'fixture',
    samples: [{ metric_name: metricName, entity_id: 'container-1', value, unit, timestamp, labels: { source: 'fixture' } }],
  }
}

describe('buildContainerOverview', () => {
  it('keeps direct use and capacity context separate for a single-series Container', () => {
    const overview = buildContainerOverview('restarting', {
      'k8s.container.cpu.usage': [series('k8s.container.cpu.usage', 35, 'millicores')],
      'k8s.container.cpu.request': [series('k8s.container.cpu.request', 100, 'millicores')],
      'k8s.container.cpu.limit': [series('k8s.container.cpu.limit', 250, 'millicores')],
      'k8s.container.memory.usage': [series('k8s.container.memory.usage', 42_000_000, 'bytes')],
      'k8s.container.memory.request': [series('k8s.container.memory.request', 64_000_000, 'bytes')],
      'k8s.container.memory.limit': [series('k8s.container.memory.limit', 128_000_000, 'bytes')],
      'k8s.container.restarts': [series('k8s.container.restarts', 3, 'count')],
      'k8s.container.ready': [series('k8s.container.ready', 1, 'count')],
    })

    expect(overview.status).toBe('Restarting')
    expect(overview.cpu).toMatchObject({ used: { kind: 'single', value: 35 }, request: { kind: 'single', value: 100 }, limit: { kind: 'single', value: 250 } })
    expect(overview.memory).toMatchObject({ used: { kind: 'single', value: 42_000_000 }, request: { kind: 'single', value: 64_000_000 }, limit: { kind: 'single', value: 128_000_000 } })
    expect(overview.restarts).toMatchObject({ kind: 'single', value: 3, cumulative: true })
    expect(overview).not.toHaveProperty('utilization')
  })

  it('marks omitted request and limit samples as not reported instead of treating them as zero', () => {
    const overview = buildContainerOverview('healthy', {
      'k8s.container.cpu.usage': [series('k8s.container.cpu.usage', 35, 'millicores')],
    })

    expect(overview.cpu.request).toEqual({ kind: 'not_reported' })
    expect(overview.cpu.limit).toEqual({ kind: 'not_reported' })
  })

  it('does not select or combine label-distinct series for a summary value', () => {
    const overview = buildContainerOverview('healthy', {
      'k8s.container.cpu.usage': [
        series('k8s.container.cpu.usage', 35, 'millicores'),
        series('k8s.container.cpu.usage', 40, 'millicores'),
      ],
    })

    expect(overview.cpu.used).toMatchObject({ kind: 'multiple', count: 2, sampleCount: 2 })
  })

  it('declares exactly the eight supported Container metrics', () => {
    expect(CONTAINER_METRIC_NAMES).toEqual([
      'k8s.container.cpu.usage',
      'k8s.container.cpu.request',
      'k8s.container.cpu.limit',
      'k8s.container.memory.usage',
      'k8s.container.memory.request',
      'k8s.container.memory.limit',
      'k8s.container.ready',
      'k8s.container.restarts',
    ])
  })
})
