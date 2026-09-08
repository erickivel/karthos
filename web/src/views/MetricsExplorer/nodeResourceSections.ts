export type NodeResourceSection = 'compute' | 'memory' | 'storage' | 'network'

export const NODE_RESOURCE_SECTIONS: Array<{ id: NodeResourceSection; label: string }> = [
  { id: 'compute', label: 'Compute' },
  { id: 'memory', label: 'Memory' },
  { id: 'storage', label: 'Storage' },
  { id: 'network', label: 'Network' },
]

const SECTION_METRICS: Record<NodeResourceSection, ReadonlySet<string>> = {
  compute: new Set(['k8s.node.cpu.usage', 'system.cpu.utilization']),
  memory: new Set(['k8s.node.memory.working_set', 'system.memory.usage']),
  storage: new Set([
    'system.disk.io.read',
    'system.disk.io.write',
    'system.disk.operations',
    'system.filesystem.usage',
    'system.filesystem.utilization',
  ]),
  network: new Set(['system.network.io.receive', 'system.network.io.transmit']),
}

/** Keeps the Node Resource page focused on one bounded metric family at a time. */
export function metricsForNodeResourceSection(
  metricNames: readonly string[],
  section: NodeResourceSection,
): string[] {
  const supported = SECTION_METRICS[section]
  return metricNames.filter((name) => supported.has(name))
}
