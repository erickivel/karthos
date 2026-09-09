import { describe, expect, it } from 'vitest'
import { metricsForNodeResourceSection } from './nodeResourceSections'

const resourceMetricNames = [
  'k8s.node.cpu.usage',
  'k8s.node.memory.working_set',
  'system.cpu.utilization',
  'system.memory.usage',
  'system.disk.io.read',
  'system.disk.io.write',
  'system.disk.operations',
  'system.filesystem.usage',
  'system.filesystem.utilization',
  'system.network.io.receive',
  'system.network.io.transmit',
]

describe('metricsForNodeResourceSection', () => {
  it('keeps the Compute section limited to the CPU summary and deferred CPU breakdown family', () => {
    expect(metricsForNodeResourceSection(resourceMetricNames, 'compute')).toEqual([
      'k8s.node.cpu.usage',
      'system.cpu.utilization',
    ])
  })

  it('keeps storage and network dimensional families out of the initial Compute section', () => {
    expect(metricsForNodeResourceSection(resourceMetricNames, 'storage')).toEqual([
      'system.disk.io.read',
      'system.disk.io.write',
      'system.disk.operations',
      'system.filesystem.usage',
      'system.filesystem.utilization',
    ])
    expect(metricsForNodeResourceSection(resourceMetricNames, 'network')).toEqual([
      'system.network.io.receive',
      'system.network.io.transmit',
    ])
  })
})
