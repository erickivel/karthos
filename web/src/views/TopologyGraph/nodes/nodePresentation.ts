import type { GraphNode } from '@/types/graph'
import { entityKindLabel } from '@/utils/entityDisplay'
import type { ProjectionMode } from '../projection'

export interface NodeSignal {
  label: string
  value: string
  tone: 'alert' | 'signal' | 'info' | 'muted'
}

export interface NodePresentation {
  kind: string
  name: string
  signal?: NodeSignal
}

export function presentEntityNode(
  node: GraphNode,
  mode: ProjectionMode | undefined,
): NodePresentation {
  const phase = node.labels?.['k8s.pod.phase']
  let signal: NodeSignal | undefined

  if (node.status === 'error' || node.status === 'critical') {
    signal = { label: 'Exception detected', value: 'CRITICAL', tone: 'alert' }
  } else if (phase === '2') {
    signal = { label: 'Status', value: 'RUNNING', tone: 'signal' }
  } else if (node.namespace && mode !== 'ownership') {
    signal = { label: 'Namespace', value: node.namespace, tone: 'info' }
  } else if (mode !== 'ownership') {
    signal = { label: 'Status', value: node.status.toUpperCase(), tone: 'muted' }
  }

  return {
    kind: entityKindLabel(node),
    name: node.name,
    signal,
  }
}
