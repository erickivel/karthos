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
  const trafficSample = {
    nodes: [
      node('ing', 'ingress', 'gateway-ingress'),
      node('svc-a', 'service', 'gateway'),
      node('svc-b', 'service', 'checkout'),
      node('workload-a', 'workload', 'gateway'),
      node('workload-b', 'workload', 'checkout'),
      node('pod-a1', 'pod', 'gateway-a1'),
      node('pod-a2', 'pod', 'gateway-a2'),
      node('pod-b1', 'pod', 'checkout-b1'),
    ],
    edges: [
      edge('routes', 'ing', 'svc-a', 'routes_to'),
      edge('exposes-a', 'svc-a', 'workload-a', 'exposes'),
      edge('exposes-b', 'svc-b', 'workload-b', 'exposes'),
      edge('contains-a1', 'workload-a', 'pod-a1', 'contains'),
      edge('contains-a2', 'workload-a', 'pod-a2', 'contains'),
      edge('contains-b1', 'workload-b', 'pod-b1', 'contains'),
      edge('svc-call', 'svc-a', 'svc-b', 'calls'),
      edge('pod-call', 'pod-a1', 'pod-b1', 'calls'),
    ],
  }

  it('Endpoint Flow shows only ingress and services with routes/calls edges', () => {
    const result = project(
      trafficSample.nodes,
      trafficSample.edges,
      'traffic',
      'overview',
    )

    expect(result.nodes.map((n) => n.id).sort()).toEqual([
      'ing',
      'svc-a',
      'svc-b',
    ])
    expect(result.edges.map((e) => e.relation).sort()).toEqual([
      'calls',
      'routes_to',
    ])
  })

  it('Workload Flow adds workloads and exposes edges, hides pods', () => {
    const result = project(
      trafficSample.nodes,
      trafficSample.edges,
      'traffic',
      'runtime',
    )

    expect(result.nodes.map((n) => n.kind).sort()).toEqual([
      'ingress',
      'service',
      'service',
      'workload',
      'workload',
    ])
    expect(result.edges.some((e) => e.relation === 'exposes')).toBe(true)
    expect(result.nodes.some((n) => n.kind === 'pod')).toBe(false)
  })

  it('Instance Flow shows only pod nodes with pod-level call edges', () => {
    const result = project(
      trafficSample.nodes,
      trafficSample.edges,
      'traffic',
      'detailed',
    )

    expect(result.nodes.map((n) => n.kind)).toEqual(['pod', 'pod'])
    expect(result.nodes.map((n) => n.id).sort()).toEqual(['pod-a1', 'pod-b1'])
    expect(result.edges.map((e) => e.relation)).toEqual(['calls'])
    expect(result.edges[0]).toMatchObject({ source: 'pod-a1', target: 'pod-b1' })
  })

  it('infrastructure detailed nests containers under pods under host nodes', () => {
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

    expect(result.nodes.map((n) => n.id).sort()).toEqual([
      'container-a',
      'host',
      'pod-a',
    ])
    expect(result.groupOf.get('pod-a')).toBe('host')
    expect(result.groupOf.get('container-a')).toBe('pod-a')
  })

  it('infrastructure overview shows only host nodes', () => {
    const result = project(
      [
        node('host', 'node', 'oksg-test-worker'),
        node('pod-a', 'pod', 'gateway-a'),
      ],
      [edge('runs-on', 'pod-a', 'host', 'runs_on')],
      'infrastructure',
      'overview',
      { includeOrphans: true },
    )

    expect(result.nodes.map((n) => n.kind)).toEqual(['node'])
  })

  it('infrastructure runtime shows pods nested under nodes (no containers)', () => {
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
      'runtime',
      { includeOrphans: true },
    )

    expect(result.nodes.map((n) => n.kind).sort()).toEqual(['node', 'pod'])
    expect(result.nodes.some((n) => n.kind === 'container')).toBe(false)
  })

  it('infrastructure drops PVC entirely', () => {
    const result = project(
      [
        node('host', 'node', 'oksg-test-worker'),
        node('pod-a', 'pod', 'gateway-a'),
        node('vol-a', 'pvc', 'data'),
      ],
      [
        edge('runs-on', 'pod-a', 'host', 'runs_on'),
        edge('mounts', 'pod-a', 'vol-a', 'mounts'),
      ],
      'infrastructure',
      'detailed',
      { includeOrphans: true },
    )

    expect(result.nodes.some((n) => n.kind === 'pvc')).toBe(false)
  })
})
