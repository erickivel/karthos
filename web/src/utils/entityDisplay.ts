import type { GraphNode } from '@/types/graph'

const WORKLOAD_TYPE_LABEL = 'k8s.workload.type'

const WORKLOAD_TYPE_LABELS: Record<string, string> = {
  deployment: 'Deployment',
  statefulset: 'StatefulSet',
  daemonset: 'DaemonSet',
  replicaset: 'ReplicaSet',
  job: 'Job',
  cronjob: 'CronJob',
  replication_controller: 'ReplicationController',
}

export function entityKindLabel(node: GraphNode): string {
  if (node.kind !== 'workload') return node.kind
  const workloadType = node.labels?.[WORKLOAD_TYPE_LABEL]
  if (!workloadType) return node.kind
  return WORKLOAD_TYPE_LABELS[workloadType] ?? workloadType
}

export function entityKindSearchText(node: GraphNode): string {
  const displayKind = entityKindLabel(node)
  if (displayKind === node.kind) return node.kind
  return `${node.kind} ${displayKind}`
}
