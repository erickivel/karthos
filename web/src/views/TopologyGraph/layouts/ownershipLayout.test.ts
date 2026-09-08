import { describe, expect, it } from 'vitest'
import type { EntityKind, GraphEdge, GraphNode } from '@/types/graph'
import { ownershipLayout } from './ownershipLayout'

function node(id: string, kind: EntityKind, name: string, namespace = ''): GraphNode {
  return {
    id,
    kind,
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
        nodes: [
          node('ns-empty', 'namespace', 'empty'),
          node('ns-demo', 'namespace', 'demo'),
          node('workload-a', 'workload', 'api', 'demo'),
        ],
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

  it('places a shared dependency below its deepest contained descendant', async () => {
    const result = await ownershipLayout.layout(
      {
        nodes: [
          node('ns-demo', 'namespace', 'demo'),
          node('service', 'service', 'api', 'demo'),
          node('deployment', 'workload', 'api', 'demo'),
          node('replicaset', 'workload', 'api-7d9c', 'demo'),
          node('pod', 'pod', 'api-7d9c-abcde', 'demo'),
          node('container', 'container', 'api', 'demo'),
          node('config', 'configmap', 'api-config', 'demo'),
        ],
        edges: [
          edge('service-exposes-deployment', 'service', 'deployment', 'exposes'),
          edge('deployment-contains-replicaset', 'deployment', 'replicaset', 'contains'),
          edge('replicaset-contains-pod', 'replicaset', 'pod', 'contains'),
          edge('pod-contains-container', 'pod', 'container', 'contains'),
          edge('deployment-uses-config', 'deployment', 'config', 'uses'),
        ],
        groupOf: new Map([
          ['service', 'ns-demo'],
          ['deployment', 'ns-demo'],
          ['replicaset', 'ns-demo'],
          ['pod', 'ns-demo'],
          ['container', 'ns-demo'],
          ['config', 'ns-demo'],
        ]),
        parentKind: 'namespace',
        nodeSizes: new Map(),
      },
      { mode: 'ownership' },
    )

    const byID = new Map(result.nodes.map((item) => [item.id, item]))
    expect(byID.get('service')?.position.y).toBeLessThan(byID.get('deployment')?.position.y ?? 0)
    expect(byID.get('deployment')?.position.y).toBeLessThan(byID.get('replicaset')?.position.y ?? 0)
    expect(byID.get('replicaset')?.position.y).toBeLessThan(byID.get('pod')?.position.y ?? 0)
    expect(byID.get('pod')?.position.y).toBeLessThan(byID.get('container')?.position.y ?? 0)
    expect(byID.get('container')?.position.y).toBeLessThan(byID.get('config')?.position.y ?? 0)
    expect(
      Math.abs(
        ((byID.get('config')?.position.x ?? 0) + (byID.get('config')?.width ?? 0) / 2) -
          ((byID.get('container')?.position.x ?? 0) + (byID.get('container')?.width ?? 0) / 2),
      ),
    ).toBeLessThan(1)
  })

  it('places a routed Service below its Ingress before the containment spine', async () => {
    const result = await ownershipLayout.layout(
      {
        nodes: [
          node('ns-demo', 'namespace', 'demo'),
          node('ingress', 'ingress', 'gateway', 'demo'),
          node('service', 'service', 'gateway', 'demo'),
          node('deployment', 'workload', 'gateway', 'demo'),
        ],
        edges: [
          edge('ingress-routes-service', 'ingress', 'service', 'routes_to'),
          edge('service-exposes-deployment', 'service', 'deployment', 'exposes'),
        ],
        groupOf: new Map([
          ['ingress', 'ns-demo'],
          ['service', 'ns-demo'],
          ['deployment', 'ns-demo'],
        ]),
        parentKind: 'namespace',
        nodeSizes: new Map(),
      },
      { mode: 'ownership' },
    )

    const byID = new Map(result.nodes.map((item) => [item.id, item]))
    expect(byID.get('ingress')?.position.y).toBeLessThan(byID.get('service')?.position.y ?? 0)
    expect(byID.get('service')?.position.y).toBeLessThan(byID.get('deployment')?.position.y ?? 0)
  })

})

function edge(id: string, source: string, target: string, relation: GraphEdge['relation']): GraphEdge {
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
