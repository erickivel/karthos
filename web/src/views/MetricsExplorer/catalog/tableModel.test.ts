import { describe, expect, it } from 'vitest'
import type { CanonicalMetric } from '@/types/metrics'
import type { CatalogTableRowModel } from './tableModel'
import { EMPTY_CATALOG_FILTERS, filterCatalogRows, sortCatalogRows } from './tableModel'

function metric(overrides: Partial<CanonicalMetric>): CanonicalMetric {
  return {
    name: 'k8s.pod.cpu.usage',
    display_name: 'Pod CPU Usage',
    unit: 'millicores',
    category: 'resource',
    scopes: ['entity'],
    description: 'Observed pod CPU usage.',
    sources: [{ id: 'kubelet', priority: 1 }],
    ...overrides,
  }
}

function row(metricValue: CanonicalMetric, samples: number, coverage: CatalogTableRowModel['rowStats']['coverage']): CatalogTableRowModel {
  return {
    metric: metricValue,
    stats: undefined,
    rowStats: {
      totalCount: samples,
      lastSeenAt: samples > 0 ? new Date(`2026-05-30T05:${String(samples).padStart(2, '0')}:00Z`) : null,
      coverage,
    },
  }
}

describe('metric catalog table model', () => {
  const rows: CatalogTableRowModel[] = [
    row(metric({
      name: 'http.server.request.count',
      display_name: 'HTTP Request Rate',
      unit: 'per_second',
      category: 'traffic',
      sources: [{ id: 'prometheus.app', priority: 1 }],
      scopes: ['entity'],
    }), 12, 'live'),
    row(metric({
      name: 'servicegraph.request.count',
      display_name: 'Service Graph Request Rate',
      unit: 'per_second',
      category: 'traffic',
      sources: [{ id: 'traces.servicegraph', priority: 1 }],
      scopes: ['entity', 'edge'],
    }), 0, 'no-data'),
    row(metric({
      name: 'k8s.container.memory.limit',
      display_name: 'Container Memory Limit',
      unit: 'bytes',
      category: 'resource',
      sources: [{ id: 'kube-cluster', priority: 1 }],
      scopes: ['entity'],
    }), 3, 'stale'),
  ]

  it('filters by search, source, category, unit, scope, and coverage', () => {
    expect(filterCatalogRows(rows, { ...EMPTY_CATALOG_FILTERS, query: 'http' }).map((r) => r.metric.name)).toEqual([
      'http.server.request.count',
    ])
    expect(filterCatalogRows(rows, { ...EMPTY_CATALOG_FILTERS, source: ['traces.servicegraph'] }).map((r) => r.metric.name)).toEqual([
      'servicegraph.request.count',
    ])
    expect(filterCatalogRows(rows, { ...EMPTY_CATALOG_FILTERS, category: ['resource'], unit: ['bytes'] }).map((r) => r.metric.name)).toEqual([
      'k8s.container.memory.limit',
    ])
    expect(filterCatalogRows(rows, { ...EMPTY_CATALOG_FILTERS, scope: ['edge'], coverage: ['no-data'] }).map((r) => r.metric.name)).toEqual([
      'servicegraph.request.count',
    ])
  })

  it('treats multiple selected values in one filter as OR choices', () => {
    expect(filterCatalogRows(rows, { ...EMPTY_CATALOG_FILTERS, source: ['traces.servicegraph', 'prometheus.app'] }).map((r) => r.metric.name)).toEqual([
      'http.server.request.count',
      'servicegraph.request.count',
    ])
    expect(filterCatalogRows(rows, { ...EMPTY_CATALOG_FILTERS, category: ['traffic'] }).map((r) => r.metric.name)).toEqual([
      'http.server.request.count',
      'servicegraph.request.count',
    ])
  })

  it('sorts with deterministic tie breakers', () => {
    expect(sortCatalogRows(rows, 'samples', 'desc').map((r) => r.metric.name)).toEqual([
      'http.server.request.count',
      'k8s.container.memory.limit',
      'servicegraph.request.count',
    ])
    expect(sortCatalogRows(rows, 'category', 'asc').map((r) => r.metric.name)).toEqual([
      'k8s.container.memory.limit',
      'http.server.request.count',
      'servicegraph.request.count',
    ])
  })
})
