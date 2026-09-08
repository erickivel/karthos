import type { CanonicalMetric, CatalogStatEntry, MetricScope } from '@/types/metrics'
import type { CatalogRowStats, CoverageState } from './coverage'

export type CatalogSortKey = 'name' | 'category' | 'samples' | 'last_seen' | 'sources'
export type CatalogSortDir = 'asc' | 'desc'

export interface CatalogFilterState {
  query: string
  source: string[]
  category: string[]
  unit: string[]
  scope: string[]
  coverage: string[]
}

export const EMPTY_CATALOG_FILTERS: CatalogFilterState = {
  query: '',
  source: [],
  category: [],
  unit: [],
  scope: [],
  coverage: [],
}

export interface CatalogTableRowModel {
  metric: CanonicalMetric
  stats: CatalogStatEntry | undefined
  rowStats: CatalogRowStats
}

function normalized(value: string | undefined | null): string {
  return (value ?? '').trim().toLowerCase()
}

function sourceIds(metric: CanonicalMetric): string[] {
  return metric.sources.map((source) => source.id)
}

function sourceFilterMatches(metric: CanonicalMetric, selected: string[]): boolean {
  if (selected.length === 0) return true
  const ids = sourceIds(metric)
  return selected.some((value) => ids.includes(value))
}

function includesSelected(values: readonly string[] | undefined, selected: string[]): boolean {
  if (selected.length === 0) return true
  return selected.some((value) => (values ?? []).includes(value))
}

function equalsSelected(value: string | undefined, selected: string[]): boolean {
  if (selected.length === 0) return true
  return selected.includes(value ?? '')
}

function queryMatches(row: CatalogTableRowModel, rawQuery: string): boolean {
  const query = normalized(rawQuery)
  if (!query) return true
  const metric = row.metric
  const haystack = [
    metric.display_name,
    metric.name,
    metric.description,
    metric.category,
    metric.unit,
    row.rowStats.coverage,
    ...(metric.scopes ?? []),
    ...(metric.applies_to ?? []),
    ...sourceIds(metric),
  ].join(' ')
  return normalized(haystack).includes(query)
}

export function filterCatalogRows(
  rows: CatalogTableRowModel[],
  filters: CatalogFilterState,
): CatalogTableRowModel[] {
  return rows.filter((row) => {
    const metric = row.metric
    return (
      queryMatches(row, filters.query) &&
      sourceFilterMatches(metric, filters.source) &&
      equalsSelected(metric.category, filters.category) &&
      equalsSelected(metric.unit, filters.unit) &&
      includesSelected(metric.scopes, filters.scope as MetricScope[]) &&
      equalsSelected(row.rowStats.coverage, filters.coverage as CoverageState[])
    )
  })
}

function compareText(a: string | undefined, b: string | undefined): number {
  return (a ?? '').localeCompare(b ?? '')
}

function tieBreak(a: CatalogTableRowModel, b: CatalogTableRowModel): number {
  const displayName = compareText(a.metric.display_name, b.metric.display_name)
  if (displayName !== 0) return displayName
  return compareText(a.metric.name, b.metric.name)
}

export function sortCatalogRows(
  rows: CatalogTableRowModel[],
  sortKey: CatalogSortKey,
  sortDir: CatalogSortDir,
): CatalogTableRowModel[] {
  const dir = sortDir === 'asc' ? 1 : -1
  return [...rows].sort((a, b) => {
    let result = 0
    switch (sortKey) {
      case 'name':
        result = tieBreak(a, b)
        break
      case 'category':
        result = compareText(a.metric.category, b.metric.category) || tieBreak(a, b)
        break
      case 'samples':
        result = a.rowStats.totalCount - b.rowStats.totalCount || tieBreak(a, b)
        break
      case 'last_seen': {
        const at = a.rowStats.lastSeenAt?.getTime() ?? 0
        const bt = b.rowStats.lastSeenAt?.getTime() ?? 0
        result = at - bt || tieBreak(a, b)
        break
      }
      case 'sources': {
        const aSources = sourceIds(a.metric).join(',')
        const bSources = sourceIds(b.metric).join(',')
        result =
          sourceIds(a.metric).length - sourceIds(b.metric).length ||
          compareText(aSources, bSources) ||
          tieBreak(a, b)
        break
      }
      default:
        result = 0
    }
    return dir * result
  })
}
