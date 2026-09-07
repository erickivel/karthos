import { describe, expect, it } from 'vitest'
import type { EntityKind, GraphNode } from '@/types/graph'
import { infrastructureLayout } from './infrastructureLayout'

function node(id: string, kind: EntityKind, name: string): GraphNode {
  return {
    id,
    kind,
    name,
    namespace: 'demo',
    cluster: 'test',
    status: 'unknown',
  }
}

describe('infrastructure nested layout', () => {
  it('keeps an incomplete container row left-aligned with the rows above it', async () => {
    const result = await infrastructureLayout.layout(
      {
        nodes: [
          node('node', 'node', 'node-a'),
          node('pod', 'pod', 'api-1'),
          node('container-a', 'container', 'app'),
          node('container-b', 'container', 'sidecar'),
          node('container-c', 'container', 'agent'),
        ],
        edges: [],
        groupOf: new Map([
          ['pod', 'node'],
          ['container-a', 'pod'],
          ['container-b', 'pod'],
          ['container-c', 'pod'],
        ]),
        parentKind: 'node',
      },
      { mode: 'infrastructure' },
    )

    const byID = new Map(result.nodes.map((item) => [item.id, item]))
    // Layout sorts by name: agent, app, then sidecar. `container-b` is the
    // lone second-row card and must remain aligned with the first column.
    const firstContainer = byID.get('container-c')
    const lastContainer = byID.get('container-b')

    expect(firstContainer).toBeDefined()
    expect(lastContainer).toBeDefined()
    expect(lastContainer?.position.x).toBe(firstContainer?.position.x)
    expect((lastContainer?.position.y ?? 0) - (firstContainer?.position.y ?? 0)).toBe(122)
  })
})
