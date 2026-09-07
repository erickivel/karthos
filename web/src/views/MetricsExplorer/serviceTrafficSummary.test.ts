import { describe, expect, it } from 'vitest'
import { summarizeServiceTraffic } from './serviceTrafficModel'

function rateSeries({
  source = 'mesh.istio',
  caller = 'gateway-go',
  destination,
  status,
  samples,
}: {
  source?: string
  caller?: string
  destination: string
  status?: string
  samples: Array<[string, number]>
}) {
  return {
    metric_name: 'servicegraph.request.count',
    unit: 'per_second',
    category: 'traffic' as const,
    description: '',
    samples: samples.map(([timestamp, value]) => ({
      metric_name: 'servicegraph.request.count',
      entity_id: 'gateway',
      value,
      timestamp,
      unit: 'per_second',
      labels: {
        source,
        edge_client: caller,
        edge_server: destination,
        ...(status ? { 'http.response.status_code': status } : {}),
      },
    })),
  }
}

describe('summarizeServiceTraffic', () => {
  it('shows one source-scoped total with destination and HTTP-outcome detail', () => {
    const summaries = summarizeServiceTraffic([
      rateSeries({ destination: 'checkout-api', status: '200', samples: [['2026-09-06T12:00:00Z', 3.9]] }),
      rateSeries({ destination: 'checkout-api', status: '500', samples: [['2026-09-06T12:00:00Z', 0.8]] }),
      rateSeries({ destination: 'inventory-api', status: '200', samples: [['2026-09-06T12:00:00Z', 1.5]] }),
      rateSeries({ destination: 'inventory-api', status: '503', samples: [['2026-09-06T12:00:00Z', 0.1]] }),
    ])

    expect(summaries).toEqual([
      {
        source: 'mesh.istio',
        timestamp: '2026-09-06T12:00:00Z',
        total: 6.3,
        outcomes: {
          successful: 5.4,
          clientFailures: 0,
          serverFailures: 0.9,
          other: 0,
        },
        destinations: [
          {
            name: 'checkout-api',
            total: 4.7,
            outcomes: { successful: 3.9, clientFailures: 0, serverFailures: 0.8, other: 0 },
            statuses: [
              { code: '200', outcome: 'successful', value: 3.9 },
              { code: '500', outcome: 'serverFailures', value: 0.8 },
            ],
          },
          {
            name: 'inventory-api',
            total: 1.6,
            outcomes: { successful: 1.5, clientFailures: 0, serverFailures: 0.1, other: 0 },
            statuses: [
              { code: '200', outcome: 'successful', value: 1.5 },
              { code: '503', outcome: 'serverFailures', value: 0.1 },
            ],
          },
        ],
      },
    ])
  })

  it('does not add values from different observed sources together', () => {
    const summaries = summarizeServiceTraffic([
      rateSeries({ source: 'mesh.istio', destination: 'checkout-api', status: '200', samples: [['2026-09-06T12:00:00Z', 2]] }),
      rateSeries({ source: 'prometheus.app', destination: 'checkout-api', status: '200', samples: [['2026-09-06T12:00:00Z', 3]] }),
    ])

    expect(summaries.map((summary) => [summary.source, summary.total])).toEqual([
      ['mesh.istio', 2],
      ['prometheus.app', 3],
    ])
  })

  it('groups inbound evidence by the calling service instead of the selected service', () => {
    const summaries = summarizeServiceTraffic([
      rateSeries({ destination: 'service-a', status: '200', samples: [['2026-09-06T12:00:00Z', 2]] }),
      rateSeries({ destination: 'service-a', status: '500', samples: [['2026-09-06T12:00:00Z', 0.5]] }),
    ], 'inbound')

    expect(summaries[0]).toMatchObject({
      total: 2.5,
      destinations: [{
        name: 'gateway-go',
        outcomes: { successful: 2, serverFailures: 0.5 },
      }],
    })
  })

  it('withholds an aggregate when a source has no single current evaluation shared by every series', () => {
    const summaries = summarizeServiceTraffic([
      rateSeries({ destination: 'checkout-api', status: '200', samples: [['2026-09-06T12:00:00Z', 2], ['2026-09-06T12:00:15Z', 4]] }),
      rateSeries({ destination: 'checkout-api', status: '500', samples: [['2026-09-06T12:00:00Z', 1]] }),
    ])

    expect(summaries).toEqual([])
  })
})
