import { describe, expect, it } from 'vitest'
import { collectAllMetricNames, getNodeMetricSpecs } from './nodeMetrics'

describe('topology node metric specs', () => {
  it('prefers servicegraph rate before HTTP rate for service cards', () => {
    const [traffic] = getNodeMetricSpecs('service')

    expect(traffic).toMatchObject({
      metric: ['servicegraph.request.count', 'http.server.request.count'],
      label: 'RATE',
    })
    expect(collectAllMetricNames()).toContain('servicegraph.request.count')
    expect(collectAllMetricNames()).toContain('http.server.request.count')
  })

  it('shows only direct pod resource metrics', () => {
    expect(getNodeMetricSpecs('pod')).toEqual([
      expect.objectContaining({ metric: 'k8s.pod.cpu.usage', label: 'CPU' }),
      expect.objectContaining({ metric: 'k8s.pod.memory.working_set', label: 'MEM' }),
    ])
    expect(getNodeMetricSpecs('workload')).toEqual([])
  })

  it('shows node usage rather than capacity estimates', () => {
    expect(getNodeMetricSpecs('node')).toEqual([
      expect.objectContaining({ metric: 'k8s.node.cpu.usage', label: 'CPU' }),
      expect.objectContaining({ metric: 'k8s.node.memory.working_set', label: 'MEM' }),
    ])
  })
})
