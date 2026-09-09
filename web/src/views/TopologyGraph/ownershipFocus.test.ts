import { describe, expect, it } from 'vitest'
import type { Edge, Node } from '@xyflow/react'
import { applyOwnershipDependencyFocus } from './ownershipFocus'

const nodes: Node[] = [
  { id: 'deployment', position: { x: 0, y: 0 }, data: { kind: 'workload' } },
  { id: 'container', position: { x: 0, y: 100 }, data: { kind: 'container' } },
  { id: 'config', position: { x: 0, y: 200 }, data: { kind: 'configmap' } },
]

const edges: Edge[] = [
  edge('contains', 'deployment', 'container', 'contains'),
  edge('uses', 'deployment', 'config', 'uses'),
  edge('mounts', 'container', 'config', 'mounts'),
]

describe('applyOwnershipDependencyFocus', () => {
  it('hides multi-parent dependency links until an ownership node is selected', () => {
    const result = applyOwnershipDependencyFocus({ nodes, edges }, 'ownership')

    expect(result.edges.map((edge) => edge.id)).toEqual(['contains'])
    expect(result.nodes.find((node) => node.id === 'deployment')?.data).toMatchObject({
      dependencyLinks: { dependencies: 1, dependents: 0 },
    })
    expect(result.nodes.find((node) => node.id === 'config')?.data).toMatchObject({
      dependencyLinks: { dependencies: 0, dependents: 2 },
    })
  })

  it('keeps dependency links out of the stable ownership canvas', () => {
    const result = applyOwnershipDependencyFocus({ nodes, edges }, 'ownership')

    expect(result.edges.map((edge) => edge.id)).toEqual(['contains'])
  })

  it('does not alter other projection modes', () => {
    const result = applyOwnershipDependencyFocus({ nodes, edges }, 'infrastructure')

    expect(result).toEqual({ nodes, edges })
  })
})

function edge(id: string, source: string, target: string, relation: string): Edge {
  return { id, source, target, data: { relation } }
}
