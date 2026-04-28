import type { EntityKind } from '@/types/graph'

export type MetricSelector = string | readonly string[]

// NodeMetricSpec — discriminated union. Adding a new visualization is one
// new variant here and one new branch in NodeMetric.tsx; nothing else
// changes.
export type NodeMetricSpec =
  | {
      kind: 'utilization'
      metric: MetricSelector
      capacityMetric?: MetricSelector
      capacityFallback?: number
      label: string
      // When true, value/capacity is interpreted as REMAINING (e.g. volume
      // available bytes); the rendered usage is (capacity - value) / capacity.
      invert?: boolean
      format?: 'cpu' | 'memory' | 'bytes' | 'percent'
    }
  | {
      kind: 'fraction'
      numeratorMetric: MetricSelector
      denominatorMetric?: MetricSelector
      denominatorMetadataKey?: string
      label: string
    }
  | {
      kind: 'rate'
      metric: MetricSelector
      unit: string
      label: string
    }
  | {
      kind: 'gauge'
      metric: MetricSelector
      label: string
      format?: 'cpu' | 'memory' | 'bytes' | 'percent'
    }

// Per-kind metric spec. Adding a metric for a kind is one entry here.
// useNodeMetrics derives the union of metric names from this map; the
// EntityNode renderer iterates the spec list. No parallel arrays.
export const NODE_METRICS: Record<EntityKind, NodeMetricSpec[]> = {
  pod: [
    {
      kind: 'utilization',
      metric: 'k8s.pod.cpu.usage',
      capacityMetric: 'k8s.node.cpu.capacity',
      capacityFallback: 1000,
      label: 'CPU',
      format: 'cpu',
    },
    {
      kind: 'utilization',
      metric: 'k8s.pod.memory.working_set',
      capacityMetric: 'k8s.node.memory.capacity',
      capacityFallback: 256 * 1024 * 1024,
      label: 'MEM',
      format: 'memory',
    },
  ],
  pvc: [
    {
      kind: 'utilization',
      metric: 'k8s.volume.available',
      capacityMetric: 'k8s.volume.capacity',
      label: 'STORAGE',
      invert: true,
      format: 'bytes',
    },
  ],
  workload: [
    {
      kind: 'fraction',
      numeratorMetric: 'k8s.deployment.available',
      denominatorMetadataKey: 'k8s.workload.replicas',
      label: 'READY',
    },
  ],
  service: [
    {
      kind: 'rate',
      metric: ['servicegraph.request.count', 'http.server.request.count'],
      unit: 'rps',
      label: 'TRAFFIC',
    },
  ],
  ingress: [
    {
      kind: 'rate',
      metric: ['servicegraph.request.count', 'http.server.request.count'],
      unit: 'rps',
      label: 'TRAFFIC',
    },
  ],
  // Kinds with no per-card metric today. Adding one is a single entry.
  cluster: [],
  node: [
    {
      kind: 'gauge',
      metric: 'k8s.node.allocatable_cpu',
      label: 'CPU',
      format: 'cpu',
    },
    {
      kind: 'gauge',
      metric: 'k8s.node.allocatable_memory',
      label: 'MEM',
      format: 'memory',
    },
  ],
  namespace: [],
  container: [],
  endpoint: [],
  configmap: [],
  secret: [],
}

function addMetricNames(names: Set<string>, selector: MetricSelector | undefined) {
  if (!selector) return
  if (typeof selector === 'string') {
    names.add(selector)
  } else {
    for (const metric of selector) names.add(metric)
  }
}

// Union of all metric names referenced by NODE_METRICS — useNodeMetrics
// uses this to build its query list.
export function collectAllMetricNames(): string[] {
  const names = new Set<string>()
  for (const specs of Object.values(NODE_METRICS)) {
    for (const spec of specs) {
      switch (spec.kind) {
        case 'utilization':
          addMetricNames(names, spec.metric)
          addMetricNames(names, spec.capacityMetric)
          break
        case 'fraction':
          addMetricNames(names, spec.numeratorMetric)
          addMetricNames(names, spec.denominatorMetric)
          break
        case 'rate':
          addMetricNames(names, spec.metric)
          break
        case 'gauge':
          addMetricNames(names, spec.metric)
          break
      }
    }
  }
  return [...names]
}

export function getNodeMetricSpecs(kind: EntityKind): NodeMetricSpec[] {
  return NODE_METRICS[kind] ?? []
}
