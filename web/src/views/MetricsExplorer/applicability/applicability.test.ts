import { describe, expect, it, beforeEach } from 'vitest'
import type { CanonicalMetric } from '@/types/metrics'
import {
  __resetApplicabilityWarnings,
  applicableMetrics,
  isMetricApplicableToKind,
  visibleCategoriesForKind,
} from './applicability'
import { KIND_CATEGORIES } from './kindCategories'

function metric(partial: Partial<CanonicalMetric>): CanonicalMetric {
  return {
    name: 'test.metric',
    display_name: 'Test',
    unit: 'count',
    category: 'resource',
    scopes: ['entity'],
    description: '',
    sources: [],
    ...partial,
  }
}

beforeEach(() => __resetApplicabilityWarnings())

describe('visibleCategoriesForKind', () => {
  it('returns the configured categories for a known kind', () => {
    expect(visibleCategoriesForKind('pod', 'entity')).toBe(KIND_CATEGORIES.pod)
    expect(visibleCategoriesForKind('configmap', 'entity')).toEqual([])
    expect(visibleCategoriesForKind('service', 'entity')).toContain('traffic')
  })

  it('returns the union of all categories when no entity is selected', () => {
    const result = visibleCategoriesForKind(null, 'entity')
    expect(result).toEqual(['resource', 'traffic', 'latency', 'error', 'capacity', 'health'])
  })

  it('always returns edge categories in edge mode regardless of kind', () => {
    expect(visibleCategoriesForKind('configmap', 'edge')).toEqual(['traffic', 'latency', 'error'])
    expect(visibleCategoriesForKind(null, 'edge')).toEqual(['traffic', 'latency', 'error'])
  })

  it('covers every EntityKind defined in the type', () => {
    // Sanity: each declared kind has an entry, even if empty.
    const kinds = Object.keys(KIND_CATEGORIES) as Array<keyof typeof KIND_CATEGORIES>
    expect(kinds).toContain('pod')
    expect(kinds).toContain('configmap')
    expect(kinds).toContain('pvc')
  })
})

describe('isMetricApplicableToKind', () => {
  it('matches when a source id maps to the kind', () => {
    const m = metric({ sources: [{ id: 'kubelet', priority: 1 }] })
    expect(isMetricApplicableToKind(m, 'pod')).toBe(true)
    expect(isMetricApplicableToKind(m, 'service')).toBe(false)
  })

  it('matches every kind for an Istio-only metric only on services', () => {
    const m = metric({ sources: [{ id: 'mesh.istio', priority: 1 }] })
    expect(isMetricApplicableToKind(m, 'service')).toBe(true)
    expect(isMetricApplicableToKind(m, 'pod')).toBe(false)
    expect(isMetricApplicableToKind(m, 'configmap')).toBe(false)
  })

  it('matches every kind when any source is wildcarded', () => {
    const m = metric({ sources: [{ id: 'kubelet', priority: 1 }, { id: 'mesh.istio', priority: 2 }] })
    expect(isMetricApplicableToKind(m, 'pod')).toBe(true)
    expect(isMetricApplicableToKind(m, 'service')).toBe(true)
  })

  it('treats unknown source ids as applicable (fail-open)', () => {
    const m = metric({ sources: [{ id: 'totally-made-up', priority: 1 }] })
    expect(isMetricApplicableToKind(m, 'pod')).toBe(true)
    expect(isMetricApplicableToKind(m, 'configmap')).toBe(true)
  })

  it('treats metrics with no sources as applicable', () => {
    const m = metric({ sources: [] })
    expect(isMetricApplicableToKind(m, 'pod')).toBe(true)
  })

  it('returns true when kind is null', () => {
    const m = metric({ sources: [{ id: 'mesh.istio', priority: 1 }] })
    expect(isMetricApplicableToKind(m, null)).toBe(true)
  })
})

describe('applicableMetrics', () => {
  const catalog: CanonicalMetric[] = [
    metric({ name: 'pod.cpu', category: 'resource', scopes: ['entity'], sources: [{ id: 'kubelet', priority: 1 }] }),
    metric({ name: 'svc.req', category: 'traffic', scopes: ['entity'], sources: [{ id: 'mesh.istio', priority: 1 }] }),
    metric({ name: 'edge.req', category: 'traffic', scopes: ['edge'], sources: [{ id: 'mesh.istio', priority: 1 }] }),
  ]

  it('keeps category + scope + applicability filtering coherent', () => {
    expect(applicableMetrics(catalog, 'resource', 'pod', 'entity').map((m) => m.name)).toEqual(['pod.cpu'])
    expect(applicableMetrics(catalog, 'traffic', 'service', 'entity').map((m) => m.name)).toEqual(['svc.req'])
    expect(applicableMetrics(catalog, 'traffic', 'configmap', 'entity')).toEqual([])
  })

  it('filters by edge scope in edge mode and ignores kind applicability', () => {
    expect(applicableMetrics(catalog, 'traffic', 'configmap', 'edge').map((m) => m.name)).toEqual(['edge.req'])
  })

  it('handles undefined catalog input', () => {
    expect(applicableMetrics(undefined, 'resource', 'pod', 'entity')).toEqual([])
  })
})
