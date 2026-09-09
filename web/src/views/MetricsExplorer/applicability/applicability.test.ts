import { describe, expect, it } from 'vitest'
import type { CanonicalMetric } from '@/types/metrics'
import { applicableMetrics, isMetricApplicableToKind, visibleCategoriesForSelection } from './applicability'

function metric(partial: Partial<CanonicalMetric>): CanonicalMetric {
  return {
    name: 'k8s.pod.cpu.usage', display_name: 'Pod CPU', unit: 'millicores',
    category: 'resource', scopes: ['entity'], description: 'CPU usage',
    applies_to: ['pod'], sources: [{ id: 'kubelet', priority: 1 }], ...partial,
  }
}

const catalog = [
  metric({ name: 'k8s.pod.cpu.usage', applies_to: ['pod'] }),
  metric({ name: 'k8s.container.ready', category: 'health', applies_to: ['container'] }),
  metric({ name: 'http.server.request.count', category: 'traffic', applies_to: ['service'] }),
  metric({ name: 'servicegraph.request.count', category: 'traffic', scopes: ['edge'], applies_to: undefined }),
]

describe('catalog-driven applicability', () => {
  it('derives entity categories from catalog kinds', () => {
    expect(visibleCategoriesForSelection(catalog, 'pod', 'entity')).toEqual(['resource'])
    expect(visibleCategoriesForSelection(catalog, 'container', 'entity')).toEqual(['health'])
    expect(visibleCategoriesForSelection(catalog, 'service', 'entity')).toEqual(['traffic'])
  })

  it('uses edge scope without applying an entity kind', () => {
    expect(applicableMetrics(catalog, 'traffic', 'pod', 'edge').map((item) => item.name))
      .toEqual(['servicegraph.request.count'])
  })

  it('accepts only a declared entity applicability kind', () => {
    expect(isMetricApplicableToKind(catalog[0]!, 'pod')).toBe(true)
    expect(isMetricApplicableToKind(catalog[0]!, 'container')).toBe(false)
  })
})
