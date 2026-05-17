import { useMemo, useState } from 'react'
import { useMetricCatalog } from '@/hooks/useMetricCatalog'
import { useMetricCatalogStats } from '@/hooks/useMetricCatalogStats'
import type { CanonicalMetric, CatalogStatEntry, MetricScope } from '@/types/metrics'
import { MetricCatalogRow } from './MetricCatalogRow'
import { computeRowStats, type CatalogRowStats } from './coverage'
import styles from './MetricCatalogTable.module.css'

type SortKey = 'name' | 'category' | 'samples' | 'last_seen' | 'sources'
type SortDir = 'asc' | 'desc'

interface MetricCatalogTableProps {
  onOpenInExplorer: (metric: CanonicalMetric, mode: 'entity' | 'edge') => void
}

interface RowModel {
  metric: CanonicalMetric
  stats: CatalogStatEntry | undefined
  rowStats: CatalogRowStats
}

function pickOpenMode(scopes: MetricScope[] | undefined): 'entity' | 'edge' {
  if (scopes && scopes.includes('entity')) return 'entity'
  if (scopes && scopes.includes('edge')) return 'edge'
  return 'entity'
}

export function MetricCatalogTable({ onOpenInExplorer }: MetricCatalogTableProps) {
  const { data: catalog, isLoading: catalogLoading } = useMetricCatalog()
  const { data: stats, isLoading: statsLoading } = useMetricCatalogStats()
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [sortKey, setSortKey] = useState<SortKey>('category')
  const [sortDir, setSortDir] = useState<SortDir>('asc')

  const rows = useMemo<RowModel[]>(() => {
    if (!catalog?.metrics) return []
    const statByName = new Map<string, CatalogStatEntry>()
    for (const s of stats?.stats ?? []) statByName.set(s.metric_name, s)
    const now = Date.now()
    return catalog.metrics.map((metric) => {
      const entry = statByName.get(metric.name)
      return { metric, stats: entry, rowStats: computeRowStats(metric, entry, now) }
    })
  }, [catalog?.metrics, stats?.stats])

  const sortedRows = useMemo(() => {
    const copy = [...rows]
    const dir = sortDir === 'asc' ? 1 : -1
    copy.sort((a, b) => {
      switch (sortKey) {
        case 'name':
          return dir * a.metric.display_name.localeCompare(b.metric.display_name)
        case 'category': {
          const c = a.metric.category.localeCompare(b.metric.category)
          return dir * (c !== 0 ? c : a.metric.display_name.localeCompare(b.metric.display_name))
        }
        case 'samples':
          return dir * (a.rowStats.totalCount - b.rowStats.totalCount)
        case 'last_seen': {
          const at = a.rowStats.lastSeenAt?.getTime() ?? 0
          const bt = b.rowStats.lastSeenAt?.getTime() ?? 0
          return dir * (at - bt)
        }
        case 'sources':
          return dir * ((a.metric.sources?.length ?? 0) - (b.metric.sources?.length ?? 0))
        default:
          return 0
      }
    })
    return copy
  }, [rows, sortKey, sortDir])

  const handleSort = (key: SortKey) => {
    if (sortKey === key) {
      setSortDir(sortDir === 'asc' ? 'desc' : 'asc')
    } else {
      setSortKey(key)
      setSortDir(key === 'samples' || key === 'last_seen' ? 'desc' : 'asc')
    }
  }

  const sortIndicator = (key: SortKey) => (sortKey === key ? (sortDir === 'asc' ? ' ▲' : ' ▼') : '')

  const toggleRow = (name: string) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  if (catalogLoading || statsLoading) {
    return <div className={styles.empty}>Loading catalog…</div>
  }
  if (!catalog?.metrics?.length) {
    return <div className={styles.empty}>No catalog metrics defined.</div>
  }

  return (
    <div className={styles.wrapper}>
      <div className={styles.summary}>
        <span>
          <strong>{rows.length}</strong> canonical metrics
        </span>
        <span className={styles.muted}>
          Coverage window: last {Math.round((stats?.lookback_seconds ?? 3600) / 60)} minutes
        </span>
      </div>
      <div className={styles.tableContainer}>
        <table className={styles.table}>
          <thead>
            <tr>
              <th aria-label="expand" />
              <th>
                <button type="button" className={styles.sortBtn} onClick={() => handleSort('name')}>
                  Name{sortIndicator('name')}
                </button>
              </th>
              <th>
                <button type="button" className={styles.sortBtn} onClick={() => handleSort('category')}>
                  Category{sortIndicator('category')}
                </button>
              </th>
              <th>Unit</th>
              <th>Scopes</th>
              <th>Applies to</th>
              <th>
                <button type="button" className={styles.sortBtn} onClick={() => handleSort('sources')}>
                  Sources{sortIndicator('sources')}
                </button>
              </th>
              <th className={styles.numericHead}>
                <button type="button" className={styles.sortBtn} onClick={() => handleSort('samples')}>
                  Samples (1h){sortIndicator('samples')}
                </button>
              </th>
              <th>
                <button type="button" className={styles.sortBtn} onClick={() => handleSort('last_seen')}>
                  Last seen{sortIndicator('last_seen')}
                </button>
              </th>
              <th>Coverage</th>
            </tr>
          </thead>
          <tbody>
            {sortedRows.map(({ metric, stats: entry, rowStats }) => {
              const openMode = pickOpenMode(metric.scopes)
              return (
                <MetricCatalogRow
                  key={metric.name}
                  metric={metric}
                  stats={entry}
                  rowStats={rowStats}
                  expanded={expanded.has(metric.name)}
                  openMode={openMode}
                  onToggle={() => toggleRow(metric.name)}
                  onOpenInExplorer={() => onOpenInExplorer(metric, openMode)}
                />
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}
