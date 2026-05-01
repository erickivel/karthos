import { describe, expect, it } from 'vitest'
import type { GraphEdge, GraphNode } from '@/types/graph'
import { project } from './projection'

const now = '2026-04-27T00:00:00Z'

function node(id: string, kind: GraphNode['kind'], name: string): GraphNode {
  return {
    id,
    kind,
    name,
    namespace: 'demo',
    cluster: 'test',
    status: 'unknown',
  }
}

function edge(id: string, source: string, target: string, relation: GraphEdge['relation']): GraphEdge {
  return {
    id,
    source,
    target,
    relation,
    confidence: 1,
    first_seen: now,
    last_seen: now,
  }
}

describe('topology projection', () => {
  it('collapses pods to overview in traffic runtime', () => {
    const result = project(
      [
        node('svc', 'service', 'gateway'),
        node('workload', 'workload', 'gateway'),
        node('pod-a', 'pod', 'gateway-a'),
        node('pod-b', 'pod', 'gateway-b'),
      ],
      [
        edge('exposes', 'svc', 'workload', 'exposes'),
        edge('contains-a', 'workload', 'pod-a', 'contains'),
        edge('contains-b', 'workload', 'pod-b', 'contains'),
      ],
      'traffic',
      'runtime',
    )

    expect(result.nodes.map((n) => n.id).sort()).toEqual(['svc', 'workload'])
  })

  it('keeps infrastructure detailed host groups compact by excluding containers', () => {
    const result = project(
      [
        node('host', 'node', 'oksg-test-worker'),
        node('pod-a', 'pod', 'gateway-a'),
        node('container-a', 'container', 'gateway'),
      ],
      [
        edge('runs-on', 'pod-a', 'host', 'runs_on'),
        edge('contains', 'pod-a', 'container-a', 'contains'),
      ],
      'infrastructure',
      'detailed',
      { includeOrphans: true },
    )

    expect(result.nodes.map((n) => n.id).sort()).toEqual(['host', 'pod-a'])
    expect(result.groupOf.get('pod-a')).toBe('host')
    expect(result.groupOf.has('container-a')).toBe(false)
  })
})
