import type { GraphSummary } from '@/types/graph'

type KpiAccent = 'signal' | 'alert' | 'info' | 'muted'

export function overviewKpis(summary: Pick<GraphSummary, 'edge_count' | 'kind_counts'> | undefined): [string, number][] {
  const kinds = summary?.kind_counts ?? {}
  return [
    ['Nodes', kinds.node ?? 0],
    ['Namespaces', kinds.namespace ?? 0],
    ['Workloads', kinds.workload ?? 0],
    ['Services', kinds.service ?? 0],
    ['Graph Edges', summary?.edge_count ?? 0],
  ]
}

export function podHealthBuckets(pods: {
  healthy: number
  pending: number
  succeeded: number
  failed: number
  unknown: number
}): [string, number, KpiAccent][] {
  return [
    ['Healthy Pods', pods.healthy, 'signal'],
    ['Pending Pods', pods.pending, pods.pending > 0 ? 'info' : 'muted'],
    ['Failed Pods', pods.failed, pods.failed > 0 ? 'alert' : 'muted'],
    ['Succeeded Pods', pods.succeeded, pods.succeeded > 0 ? 'info' : 'muted'],
    ['Unknown Pods', pods.unknown, pods.unknown > 0 ? 'info' : 'muted'],
  ]
}
