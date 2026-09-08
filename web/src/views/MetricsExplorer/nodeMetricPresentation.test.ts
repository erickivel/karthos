import { describe, expect, it } from 'vitest'
import { metricPresentation } from './metricPresentation'

describe('node metric presentation', () => {
  it.each([
    ['k8s.node.cpu.usage', {}, 'CPU used'],
    ['k8s.node.memory.working_set', {}, 'Working memory'],
    ['system.cpu.utilization', { cpu: 'cpu0', state: 'idle' }, 'CPU idle time · cpu0'],
    ['system.memory.usage', { state: 'cached' }, 'Memory · cached'],
    ['system.disk.io.read', { device: 'sda', direction: 'read' }, 'Disk read throughput · sda'],
    ['system.disk.io.write', { device: 'sda', direction: 'write' }, 'Disk write throughput · sda'],
    ['system.disk.operations', { device: 'sda', direction: 'write' }, 'Disk write operations · sda'],
    ['system.filesystem.usage', { mountpoint: '/', state: 'used' }, 'Filesystem used · /'],
    ['system.filesystem.utilization', { mountpoint: '/var' }, 'Filesystem utilization · /var'],
    ['system.network.io.receive', { network_interface_name: 'cni0', direction: 'receive' }, 'Network received throughput · cni0'],
    ['system.network.io.transmit', { network_interface_name: 'cni0', direction: 'transmit' }, 'Network sent throughput · cni0'],
    ['system.network.errors', { network_interface_name: 'cni0', direction: 'receive' }, 'Network received errors · cni0'],
    ['k8s.node.condition.ready', {}, 'Node readiness'],
    ['k8s.node.condition.memory_pressure', {}, 'Memory pressure'],
    ['k8s.node.condition.disk_pressure', {}, 'Disk pressure'],
  ])('uses a plain-language title for %s', (canonicalName, labels, title) => {
    expect(metricPresentation({
      canonicalName,
      scope: 'entity',
      labels,
      fallbackTitle: canonicalName,
    }).title).toBe(title)
  })

  it('shows only the labels that distinguish a host CPU state series', () => {
    expect(metricPresentation({
      canonicalName: 'system.cpu.utilization',
      scope: 'entity',
      labels: { cpu: 'cpu0', state: 'idle', source: 'host-os' },
    }).seriesLabels).toEqual([
      { label: 'CPU core', value: 'cpu0' },
      { label: 'CPU state', value: 'idle' },
    ])
  })
})
