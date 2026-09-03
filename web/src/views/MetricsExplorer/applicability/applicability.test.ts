import { describe, expect, it, beforeEach } from 'vitest'
import type { CanonicalMetric } from '@/types/metrics'
import {
  __resetApplicabilityWarnings,
  applicableMetrics,
  isMetricApplicableToKind,
  visibleCategoriesForSelection,
} from './applicability'

function metric(partial: Partial<CanonicalMetric>): CanonicalMetric {
  return {
    name: 'test.metric',
    display_name: 'Test',
    unit: 'count',
    category: 'resource',
    scopes: ['entity'],
    description: '',
    sources: [],
    applies_to: ['pod'],
    ...partial,
  }
}

beforeEach(() => __resetApplicabilityWarnings())

describe('visibleCategoriesForSelection', () => {
  const catalog: CanonicalMetric[] = [
    metric({ name: 'pod.cpu', category: 'resource', applies_to: ['pod'] }),
    metric({ name: 'pod.health', category: 'health', applies_to: ['pod'] }),
    metric({ name: 'container.cpu', category: 'resource', applies_to: ['container'] }),
    metric({ name: 'container.health', category: 'health', applies_to: ['container'] }),
    metric({ name: 'svc.req', category: 'traffic', applies_to: ['service'] }),
    metric({ name: 'edge.req', category: 'traffic', scopes: ['edge'], applies_to: undefined }),
  ]

  it('derives entity tabs from applicable catalog metrics', () => {
    expect(visibleCategoriesForSelection(catalog, 'pod', 'entity')).toEqual(['resource', 'health'])
    expect(visibleCategoriesForSelection(catalog, 'container', 'entity')).toEqual(['resource', 'health'])
    expect(visibleCategoriesForSelection(catalog, 'configmap', 'entity')).toEqual([])
    expect(visibleCategoriesForSelection(catalog, 'service', 'entity')).toEqual(['traffic'])
  })

  it('returns the union of all categories when no entity is selected', () => {
    const result = visibleCategoriesForSelection(catalog, null, 'entity')
    expect(result).toEqual(['resource', 'traffic', 'latency', 'error', 'capacity', 'health'])
  })

  it('derives edge tabs from edge-scoped catalog metrics', () => {
    expect(visibleCategoriesForSelection(catalog, 'configmap', 'edge')).toEqual(['traffic'])
    expect(visibleCategoriesForSelection(catalog, null, 'edge')).toEqual(['traffic'])
  })

  it('does not expose chart categories in catalog mode', () => {
    expect(visibleCategoriesForSelection(catalog, null, 'catalog')).toEqual([])
    expect(visibleCategoriesForSelection(undefined, null, 'catalog')).toEqual([])
  })

  it('uses broad defaults while the catalog is loading', () => {
    expect(visibleCategoriesForSelection(undefined, 'container', 'entity')).toEqual([
      'resource',
      'traffic',
      'latency',
      'error',
      'capacity',
      'health',
    ])
    expect(visibleCategoriesForSelection(undefined, null, 'edge')).toEqual(['traffic', 'latency', 'error'])
  })
})

describe('isMetricApplicableToKind', () => {
  it('matches when applies_to includes the selected kind', () => {
    const m = metric({ applies_to: ['pod'] })
    expect(isMetricApplicableToKind(m, 'pod')).toBe(true)
    expect(isMetricApplicableToKind(m, 'service')).toBe(false)
  })

  it('uses backend applicability instead of source ids', () => {
    const m = metric({ sources: [{ id: 'kubelet', priority: 1 }], applies_to: ['container'] })
    expect(isMetricApplicableToKind(m, 'container')).toBe(true)
    expect(isMetricApplicableToKind(m, 'pod')).toBe(false)
    expect(isMetricApplicableToKind(m, 'pvc')).toBe(false)
  })

  it('supports service-scoped traffic metrics', () => {
    const m = metric({ sources: [{ id: 'mesh.istio', priority: 1 }], applies_to: ['service'] })
    expect(isMetricApplicableToKind(m, 'service')).toBe(true)
    expect(isMetricApplicableToKind(m, 'pod')).toBe(false)
    expect(isMetricApplicableToKind(m, 'configmap')).toBe(false)
  })

  it('treats missing applies_to as applicable for older catalog responses', () => {
    const m = metric({ applies_to: undefined })
    expect(isMetricApplicableToKind(m, 'pod')).toBe(true)
    expect(isMetricApplicableToKind(m, 'configmap')).toBe(true)
  })

  it('returns true when kind is null', () => {
    const m = metric({ applies_to: ['service'] })
    expect(isMetricApplicableToKind(m, null)).toBe(true)
  })

  it('filters controller workload metrics by workload subtype', () => {
    const deploymentMetric = metric({ name: 'k8s.deployment.available', applies_to: ['workload'] })
    const jobMetric = metric({ name: 'k8s.job.active_pods', applies_to: ['workload'] })

    expect(isMetricApplicableToKind(deploymentMetric, 'workload', {
      labels: { 'k8s.workload.type': 'deployment' },
    })).toBe(true)
    expect(isMetricApplicableToKind(jobMetric, 'workload', {
      labels: { 'k8s.workload.type': 'deployment' },
    })).toBe(false)
    expect(isMetricApplicableToKind(jobMetric, 'workload', {
      labels: { 'k8s.workload.type': 'job' },
    })).toBe(true)
  })

  it('fails open for workload nodes without subtype labels', () => {
    const jobMetric = metric({ name: 'k8s.job.active_pods', applies_to: ['workload'] })
    expect(isMetricApplicableToKind(jobMetric, 'workload')).toBe(true)
  })
})

describe('applicableMetrics', () => {
  const catalog: CanonicalMetric[] = [
    metric({ name: 'pod.cpu', category: 'resource', scopes: ['entity'], applies_to: ['pod'] }),
    metric({ name: 'container.cpu', category: 'resource', scopes: ['entity'], applies_to: ['container'] }),
    metric({ name: 'container.health', category: 'health', scopes: ['entity'], applies_to: ['container'] }),
    metric({ name: 'node.cpu', category: 'resource', scopes: ['entity'], applies_to: ['node'] }),
    metric({ name: 'volume.inodes', category: 'resource', scopes: ['entity'], applies_to: ['pvc'] }),
    metric({ name: 'svc.req', category: 'traffic', scopes: ['entity'], applies_to: ['service'] }),
    metric({ name: 'edge.req', category: 'traffic', scopes: ['edge'], applies_to: undefined }),
    metric({ name: 'k8s.deployment.available', category: 'health', scopes: ['entity'], applies_to: ['workload'] }),
    metric({ name: 'k8s.job.active_pods', category: 'health', scopes: ['entity'], applies_to: ['workload'] }),
  ]

  it('keeps category + scope + applicability filtering coherent', () => {
    expect(applicableMetrics(catalog, 'resource', 'pod', 'entity').map((m) => m.name)).toEqual(['pod.cpu'])
    expect(applicableMetrics(catalog, 'traffic', 'service', 'entity').map((m) => m.name)).toEqual(['svc.req'])
    expect(applicableMetrics(catalog, 'traffic', 'configmap', 'entity')).toEqual([])
  })

  it('shows container health and excludes pod, node, and PVC metrics for containers', () => {
    expect(applicableMetrics(catalog, 'resource', 'container', 'entity').map((m) => m.name)).toEqual(['container.cpu'])
    expect(applicableMetrics(catalog, 'health', 'container', 'entity').map((m) => m.name)).toEqual([
      'container.health',
    ])
  })

  it('filters by edge scope in edge mode and ignores kind applicability', () => {
    expect(applicableMetrics(catalog, 'traffic', 'configmap', 'edge').map((m) => m.name)).toEqual(['edge.req'])
  })

  it('does not return sample metrics in catalog mode', () => {
    expect(applicableMetrics(catalog, 'traffic', null, 'catalog')).toEqual([])
  })

  it('handles undefined catalog input', () => {
    expect(applicableMetrics(undefined, 'resource', 'pod', 'entity')).toEqual([])
  })

  it('shows only metrics for the selected workload subtype', () => {
    expect(applicableMetrics(catalog, 'health', 'workload', 'entity', {
      labels: { 'k8s.workload.type': 'deployment' },
    }).map((m) => m.name)).toEqual(['k8s.deployment.available'])
    expect(applicableMetrics(catalog, 'health', 'workload', 'entity', {
      labels: { 'k8s.workload.type': 'job' },
    }).map((m) => m.name)).toEqual(['k8s.job.active_pods'])
  })

})
