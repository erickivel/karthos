import { describe, expect, it } from 'vitest'
import type { GraphNode } from '@/types/graph'
import { entityKindLabel, entityKindSearchText } from './entityDisplay'

function node(overrides: Partial<GraphNode>): GraphNode {
  return {
    id: 'node-1',
    kind: 'workload',
    name: 'api-7f44c55989',
    namespace: 'default',
    cluster: 'test',
    status: 'unknown',
    ...overrides,
  }
}

describe('entity display helpers', () => {
  it('shows workload subtype labels when structural discovery provides them', () => {
    expect(entityKindLabel(node({ labels: { 'k8s.workload.type': 'replicaset' } }))).toBe('ReplicaSet')
    expect(entityKindLabel(node({ labels: { 'k8s.workload.type': 'statefulset' } }))).toBe('StatefulSet')
  })

  it('keeps generic workload labels when subtype metadata is absent', () => {
    expect(entityKindLabel(node({ labels: {} }))).toBe('workload')
  })

  it('keeps workload searchable by both generic kind and subtype', () => {
    expect(entityKindSearchText(node({ labels: { 'k8s.workload.type': 'daemonset' } }))).toBe('workload DaemonSet')
  })

  it('leaves non-workload kinds unchanged', () => {
    expect(entityKindLabel(node({ kind: 'pod', labels: { 'k8s.workload.type': 'replicaset' } }))).toBe('pod')
  })
})
