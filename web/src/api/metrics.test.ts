import { afterEach, describe, expect, it, vi } from 'vitest'
import { getAllSeries, getSeries } from './metrics'

const query = {
  aggregation: 'latest',
  from: '2026-09-06T12:00:00Z',
  to: '2026-09-06T12:15:00Z',
  step: '15s',
  timestamp_semantics: 'evaluation',
} as const

function page(cpu: string, nextToken?: string) {
  return {
    data: {
      query,
      total: 2,
      series: [{
        metric_name: 'system.cpu.utilization',
        unit: 'percent',
        category: 'resource',
        description: 'CPU state time.',
        samples: [{
          metric_name: 'system.cpu.utilization',
          entity_id: 'node-1',
          value: 90,
          timestamp: '2026-09-06T12:15:00Z',
          unit: 'percent',
          labels: { cpu, state: 'idle', source: 'host-os' },
        }],
      }],
    },
    ...(nextToken ? { next_token: nextToken } : {}),
  }
}

describe('metric series client', () => {
  afterEach(() => vi.unstubAllGlobals())

  it('walks every cursor page for a complete dimensional CPU projection', async () => {
    vi.stubGlobal('window', { location: { origin: 'http://frontend.test' } })
    const fetchMock = vi.fn()
      .mockResolvedValueOnce(new Response(JSON.stringify(page('cpu0', 'next-page')), { status: 200 }))
      .mockResolvedValueOnce(new Response(JSON.stringify(page('cpu1')), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    const result = await getAllSeries({
      metric_name: 'system.cpu.utilization',
      entity_id: ['node-1'],
      page_size: 50,
    })

    expect(result.series.map((entry) => entry.samples[0]?.labels?.cpu)).toEqual(['cpu0', 'cpu1'])
    expect(result.total).toBe(2)
    expect(result.next_token).toBeUndefined()
    expect(new URL(fetchMock.mock.calls[1]?.[0] as string).searchParams.get('next_token')).toBe('next-page')
  })

  it('keeps a regular card on its current cursor page', async () => {
    vi.stubGlobal('window', { location: { origin: 'http://frontend.test' } })
    const fetchMock = vi.fn().mockResolvedValue(new Response(JSON.stringify(page('cpu0', 'next-page')), { status: 200 }))
    vi.stubGlobal('fetch', fetchMock)

    const result = await getSeries({
      metric_name: 'system.disk.io.read',
      entity_id: ['node-1'],
      page_size: 50,
      next_token: 'current-page',
    })

    expect(result.series).toHaveLength(1)
    expect(result.next_token).toBe('next-page')
    expect(new URL(fetchMock.mock.calls[0]?.[0] as string).searchParams.get('next_token')).toBe('current-page')
  })
})
