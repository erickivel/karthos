import { describe, expect, it } from 'vitest'
import { collectAllMetricNames, getNodeMetricSpecs } from './nodeMetrics'

describe('topology node metric specs', () => {
  it('fetches servicegraph traffic before HTTP fallback for service cards', () => {
    const [traffic] = getNodeMetricSpecs('service')

    expect(traffic).toMatchObject({
      kind: 'rate',
      metric: ['servicegraph.request.count', 'http.server.request.count'],
      label: 'TRAFFIC',
    })
    expect(collectAllMetricNames()).toContain('servicegraph.request.count')
    expect(collectAllMetricNames()).toContain('http.server.request.count')
  })

  it('uses workload metadata as the desired replica denominator', () => {
    const [ready] = getNodeMetricSpecs('workload')

    expect(ready).toMatchObject({
      kind: 'fraction',
      numeratorMetric: 'k8s.deployment.available',
      denominatorMetadataKey: 'k8s.workload.replicas',
      label: 'READY',
    })
    expect(collectAllMetricNames()).not.toContain('k8s.replicaset.desired')
  })

  it('shows node capacity chips from collected node allocatable metrics', () => {
    expect(getNodeMetricSpecs('node')).toEqual([
      expect.objectContaining({ kind: 'gauge', metric: 'k8s.node.allocatable_cpu', label: 'CPU' }),
      expect.objectContaining({ kind: 'gauge', metric: 'k8s.node.allocatable_memory', label: 'MEM' }),
    ])
  })
})
