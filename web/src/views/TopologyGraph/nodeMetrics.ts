import type { EntityKind } from '@/types/graph'

export type MetricSelector = string | readonly string[]

export interface NodeMetricSpec {
  metric: MetricSelector
  label: string
  format: 'cpu' | 'memory' | 'rate'
}

// The topology presents only current, directly measured resource use plus a
// service-rate hint. Every metric remains fully explorable through the catalog.
export const NODE_METRICS: Record<EntityKind, NodeMetricSpec[]> = {
  pod: [
    { metric: 'k8s.pod.cpu.usage', label: 'CPU', format: 'cpu' },
    { metric: 'k8s.pod.memory.working_set', label: 'MEM', format: 'memory' },
  ],
  node: [
    { metric: 'k8s.node.cpu.usage', label: 'CPU', format: 'cpu' },
    { metric: 'k8s.node.memory.working_set', label: 'MEM', format: 'memory' },
  ],
  service: [
    { metric: ['servicegraph.request.count', 'http.server.request.count'], label: 'RATE', format: 'rate' },
  ],
  cluster: [], namespace: [], workload: [], container: [], ingress: [], endpoint: [],
  configmap: [], secret: [], pvc: [], hpa: [],
}

export function collectAllMetricNames(): string[] {
  const names = new Set<string>()
  for (const specs of Object.values(NODE_METRICS)) {
    for (const spec of specs) {
      if (typeof spec.metric === 'string') names.add(spec.metric)
      else spec.metric.forEach((name) => names.add(name))
    }
  }
  return [...names]
}

export function getNodeMetricSpecs(kind: EntityKind): NodeMetricSpec[] {
  return NODE_METRICS[kind]
}
