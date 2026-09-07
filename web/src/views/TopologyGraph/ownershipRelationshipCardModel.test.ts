import { describe, expect, it } from 'vitest'
import type { GraphEdge, GraphNode } from '@/types/graph'
import { relationshipEntriesFor } from './ownershipRelationshipCardModel'

const nodes: GraphNode[] = [
  graphNode('workload', 'workload', 'coredns'),
  graphNode('config', 'configmap', 'coredns'),
  graphNode('secret', 'secret', 'coredns-tls'),
]

describe('relationshipEntriesFor', () => {
  it('lists a selected workload’s direct ownership dependencies by relation', () => {
    const entries = relationshipEntriesFor(nodes, [
      graphEdge('uses-config', 'workload', 'config', 'uses'),
      graphEdge('mounts-secret', 'workload', 'secret', 'mounts'),
      graphEdge('contains', 'workload', 'config', 'contains'),
    ], 'workload')

    expect(entries).toEqual([
      { id: 'uses-config', relation: 'uses', direction: 'outbound', related: nodes[1] },
      { id: 'mounts-secret', relation: 'mounts', direction: 'outbound', related: nodes[2] },
    ])
  })

  it('describes dependency consumers when the resource itself is selected', () => {
    const entries = relationshipEntriesFor(nodes, [
      graphEdge('uses-config', 'workload', 'config', 'uses'),
    ], 'config')

    expect(entries).toEqual([
      { id: 'uses-config', relation: 'uses', direction: 'inbound', related: nodes[0] },
    ])
  })

  it('excludes non-dependency relationships and missing related entities', () => {
    const entries = relationshipEntriesFor(nodes, [
      graphEdge('contains', 'workload', 'config', 'contains'),
      graphEdge('unknown-resource', 'workload', 'missing', 'uses'),
    ], 'workload')

    expect(entries).toEqual([])
  })
})

function graphNode(id: string, kind: GraphNode['kind'], name: string): GraphNode {
  return { id, kind, name, cluster: 'test', namespace: 'kube-system', status: 'unknown' }
}

function graphEdge(
  id: string,
  source: string,
  target: string,
  relation: GraphEdge['relation'],
): GraphEdge {
  return {
    id,
    source,
    target,
    relation,
    confidence: 1,
    provenance: ['test'],
    first_seen: '2026-01-01T00:00:00Z',
    last_seen: '2026-01-01T00:00:00Z',
  }
}
