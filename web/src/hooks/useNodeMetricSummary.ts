import { useMemo } from 'react'
import { useQueries } from '@tanstack/react-query'
import { getSamples } from '@/api/metrics'
import type { GraphNode } from '@/types/graph'

export interface NodeMetricSummaryItem {
  label: string
  value: string
  tone?: 'signal' | 'info' | 'alert' | 'muted'
}

const METRICS_BY_KIND: Partial<Record<GraphNode['kind'], string[]>> = {
  node: [
    'node_condition_ready',
    'node_condition_memory_pressure',
    'node_condition_disk_pressure',
  ],
  namespace: ['namespace_phase'],
  workload: [
    'deployment_available',
    'daemonset_ready_nodes',
    'statefulset_ready_pods',
    'replica_count',
  ],
  pod: ['pod_phase'],
}

function formatSample(metricName: string, value: number): NodeMetricSummaryItem | null {
  switch (metricName) {
    case 'node_condition_ready':
      return {
        label: 'Ready',
        value: value >= 1 ? 'Ready' : 'Not Ready',
        tone: value >= 1 ? 'signal' : 'alert',
      }
    case 'node_condition_memory_pressure':
      return {
        label: 'Memory Pressure',
        value: value >= 1 ? 'Detected' : 'Clear',
        tone: value >= 1 ? 'alert' : 'signal',
      }
    case 'node_condition_disk_pressure':
      return {
        label: 'Disk Pressure',
        value: value >= 1 ? 'Detected' : 'Clear',
        tone: value >= 1 ? 'alert' : 'signal',
      }
    case 'namespace_phase':
      return {
        label: 'Phase',
        value: value >= 1 ? 'Active' : 'Terminating',
        tone: value >= 1 ? 'signal' : 'alert',
      }
    case 'deployment_available':
      return {
        label: 'Available Replicas',
        value: Math.round(value).toLocaleString(),
        tone: value > 0 ? 'signal' : 'muted',
      }
    case 'daemonset_ready_nodes':
      return {
        label: 'Ready Nodes',
        value: Math.round(value).toLocaleString(),
        tone: value > 0 ? 'signal' : 'muted',
      }
    case 'statefulset_ready_pods':
      return {
        label: 'Ready Pods',
        value: Math.round(value).toLocaleString(),
        tone: value > 0 ? 'signal' : 'muted',
      }
    case 'replica_count':
      return {
        label: 'Desired Replicas',
        value: Math.round(value).toLocaleString(),
        tone: value > 0 ? 'info' : 'muted',
      }
    case 'pod_phase': {
      const phaseName =
        value === 1 ? 'Running'
        : value === 2 ? 'Pending'
        : value === 3 ? 'Succeeded'
        : value === 4 ? 'Failed'
        : 'Unknown'
      return {
        label: 'Phase',
        value: phaseName,
        tone:
          phaseName === 'Running' ? 'signal'
          : phaseName === 'Pending' ? 'info'
          : phaseName === 'Failed' ? 'alert'
          : 'muted',
      }
    }
    default:
      return null
  }
}

export function useNodeMetricSummary(node: GraphNode | null | undefined): NodeMetricSummaryItem[] {
  const metricNames = node ? (METRICS_BY_KIND[node.kind] ?? []) : []

  const queries = useQueries({
    queries: metricNames.map((metricName) => ({
      queryKey: ['node-metric-summary', node?.id, metricName],
      queryFn: () => getSamples({ metric_name: metricName, entity_id: node ? [node.id] : [], page_size: 1 }),
      enabled: Boolean(node),
      staleTime: 15_000,
      refetchInterval: 30_000,
    })),
  })

  return useMemo(
    () =>
      queries.flatMap((query, index) => {
        const sample = query.data?.samples[0]
        const metricName = metricNames[index]
        if (!sample || !metricName) return []

        const item = formatSample(metricName, sample.value)
        return item ? [item] : []
      }),
    [metricNames, queries],
  )
}
