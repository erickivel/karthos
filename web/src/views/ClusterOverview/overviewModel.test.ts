import { describe, expect, it } from 'vitest'
import { overviewKpis, podHealthBuckets } from './overviewModel'

describe('overview model', () => {
  it('derives entity counts from the graph summary instead of discovery', () => {
    expect(overviewKpis({
      edge_count: 52,
      kind_counts: { node: 2, namespace: 4, workload: 5, service: 6 },
    })).toEqual([
      ['Nodes', 2],
      ['Namespaces', 4],
      ['Workloads', 5],
      ['Services', 6],
      ['Graph Edges', 52],
    ])
  })

  it('renders only pod health buckets returned by the status contract', () => {
    expect(podHealthBuckets({ healthy: 12, pending: 1, succeeded: 2, failed: 0, unknown: 3 }))
      .toEqual([
        ['Healthy Pods', 12, 'signal'],
        ['Pending Pods', 1, 'info'],
        ['Failed Pods', 0, 'muted'],
        ['Succeeded Pods', 2, 'info'],
        ['Unknown Pods', 3, 'info'],
      ])
  })
})
