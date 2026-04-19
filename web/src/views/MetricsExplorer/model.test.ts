import { describe, expect, it } from 'vitest'
import { buildEntityOptions, filterMetricsForMode } from './model'
import type { GraphNode } from '@/types/graph'
import type { CanonicalMetric } from '@/types/metrics'

describe('MetricsExplorer model', () => {
  it('filters edge mode down to edge-capable metrics only', () => {
    const metrics: CanonicalMetric[] = [
      {
        name: 'request_rate_per_second',
        unit: 'per_second',
        category: 'traffic',
        scopes: ['entity', 'edge'],
        description: 'rate',
      },
      {
        name: 'net_rx_bytes',
        unit: 'bytes',
        category: 'traffic',
        scopes: ['entity'],
        description: 'rx bytes',
      },
    ]

    expect(filterMetricsForMode(metrics, 'traffic', 'edge').map((metric) => metric.name)).toEqual([
      'request_rate_per_second',
    ])
  })

  it('builds entity options from live graph nodes and sorts them by kind, namespace, and name', () => {
    const nodes: GraphNode[] = [
      {
        id: 'stale-id',
        kind: 'workload',
        namespace: 'zeta',
        name: 'old-rollout',
        cluster: '',
        status: 'unknown',
      },
      {
        id: 'service-a',
        kind: 'service',
        namespace: 'alpha',
        name: 'api',
        cluster: '',
        status: 'ready',
      },
      {
        id: 'pod-a',
        kind: 'pod',
        namespace: 'alpha',
        name: 'api-7b4c',
        cluster: '',
        status: 'ready',
      },
    ]

    expect(buildEntityOptions(nodes)).toEqual([
      { id: 'pod-a', label: '[pod] alpha/api-7b4c' },
      { id: 'service-a', label: '[service] alpha/api' },
      { id: 'stale-id', label: '[workload] zeta/old-rollout' },
    ])
  })
})
