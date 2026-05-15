import { describe, expect, it } from 'vitest'
import type { GraphNode } from '@/types/graph'
import { ownershipLayout } from './ownershipLayout'

function node(id: string, name: string, namespace = ''): GraphNode {
  return {
    id,
    kind: namespace ? 'workload' : 'namespace',
    name,
    namespace,
    cluster: 'test',
    status: 'unknown',
  }
}

describe('ownership layout', () => {
  it('renders visible empty namespaces as namespace containers', async () => {
    const result = await ownershipLayout.layout(
      {
        nodes: [node('ns-empty', 'empty'), node('ns-demo', 'demo'), node('workload-a', 'api', 'demo')],
        edges: [],
        groupOf: new Map([['workload-a', 'ns-demo']]),
        parentKind: 'namespace',
        nodeSizes: new Map(),
      },
      { mode: 'ownership' },
    )

    const empty = result.nodes.find((n) => n.id === 'ns-empty')
    const demo = result.nodes.find((n) => n.id === 'ns-demo')
    const workload = result.nodes.find((n) => n.id === 'workload-a')

    expect(empty?.type).toBe('namespace')
    expect(empty?.width).toBeGreaterThan(0)
    expect(empty?.height).toBeGreaterThan(0)
    expect(demo?.type).toBe('namespace')
    expect(workload?.type).toBe('entity')
  })
})
