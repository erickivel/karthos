import { describe, expect, it } from 'vitest'
import { hasCurrentCallsEdge } from './model'
import type { GraphEdge } from '@/types/graph'

describe('MetricsExplorer model', () => {
  it('accepts an edge selection only while its calls relation is in the current graph', () => {
    const edges: GraphEdge[] = [
      {
        id: 'gateway-calls-service', source: 'gateway', target: 'service', relation: 'calls',
        confidence: 1, first_seen: '2026-09-06T15:00:00Z', last_seen: '2026-09-06T15:00:00Z', metadata: {},
      },
      {
        id: 'service-uses-config', source: 'service', target: 'config', relation: 'uses',
        confidence: 1, first_seen: '2026-09-06T15:00:00Z', last_seen: '2026-09-06T15:00:00Z', metadata: {},
      },
    ]

    expect(hasCurrentCallsEdge(edges, 'gateway', 'service')).toBe(true)
    expect(hasCurrentCallsEdge(edges, 'service', 'config')).toBe(false)
    expect(hasCurrentCallsEdge(edges, 'past-client', 'past-server')).toBe(false)
  })
})
