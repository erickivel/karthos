import { describe, expect, it } from 'vitest'
import type { GraphNode } from '@/types/graph'
import { presentEntityNode } from './nodePresentation'

function node(overrides: Partial<GraphNode> = {}): GraphNode {
  return {
    id: 'workload-gateway',
    kind: 'workload',
    name: 'gateway-go',
    namespace: 'oksg-demo',
    cluster: 'remote-k3s',
    status: 'unknown',
    labels: { 'k8s.workload.type': 'deployment' },
    ...overrides,
  }
}

describe('presentEntityNode', () => {
  it('keeps the canonical kind and Kubernetes name distinct', () => {
    expect(presentEntityNode(node(), 'traffic')).toMatchObject({
      kind: 'Deployment',
      name: 'gateway-go',
    })
  })

  it('does not repeat a child namespace inside the ownership namespace group', () => {
    expect(presentEntityNode(node(), 'ownership').signal).toBeUndefined()
  })

  it('keeps the namespace signal in ungrouped views', () => {
    expect(presentEntityNode(node(), 'traffic').signal).toEqual({
      label: 'Namespace',
      value: 'oksg-demo',
      tone: 'info',
    })
  })
})
