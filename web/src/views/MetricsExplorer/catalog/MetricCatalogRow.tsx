import type { KeyboardEvent } from 'react'
import type { CanonicalMetric, CatalogStatEntry } from '@/types/metrics'
import { CoverageBadge } from './CoverageBadge'
import type { CatalogRowStats } from './coverage'
import styles from './MetricCatalogRow.module.css'

interface MetricCatalogRowProps {
  metric: CanonicalMetric
  stats: CatalogStatEntry | undefined
  rowStats: CatalogRowStats
  expanded: boolean
  openMode: 'entity' | 'edge'
  onToggle: () => void
  onOpenInExplorer: () => void
}

function formatLastSeen(ts: Date | null): string {
  if (!ts) return '—'
  const seconds = Math.floor((Date.now() - ts.getTime()) / 1000)
  if (seconds < 60) return `${seconds}s ago`
  const minutes = Math.floor(seconds / 60)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  return `${hours}h ${minutes % 60}m ago`
}

export function MetricCatalogRow({
  metric,
  stats,
  rowStats,
  expanded,
  openMode,
  onToggle,
  onOpenInExplorer,
}: MetricCatalogRowProps) {
  const sources = metric.sources
  const appliesTo = metric.applies_to ?? []

  // Whole-row click toggles expansion. The chevron is now a visual indicator
  // only — it inherits the row's click behavior. Keyboard support via Enter
  // and Space on the row's role="button" affordance.
  const handleRowKeyDown = (e: KeyboardEvent<HTMLTableRowElement>) => {
    if (e.key === 'Enter' || e.key === ' ') {
      e.preventDefault()
      onToggle()
    }
  }

  return (
    <>
      <tr
        className={styles.row}
        data-expanded={expanded}
        role="button"
        tabIndex={0}
        aria-expanded={expanded}
        aria-label={expanded ? 'Collapse details' : 'Expand details'}
        onClick={onToggle}
        onKeyDown={handleRowKeyDown}
      >
        <td className={styles.toggleCell} aria-hidden>
          <span className={styles.toggle}>{expanded ? '▾' : '▸'}</span>
        </td>
        <td>
          <div className={styles.nameCell}>
            <span className={styles.displayName}>{metric.display_name}</span>
            <code className={styles.canonical}>{metric.name}</code>
          </div>
        </td>
        <td>{metric.category}</td>
        <td className={styles.muted}>{metric.unit || '—'}</td>
        <td>
          <div className={styles.chipRow}>
            {(metric.scopes ?? []).map((s) => (
              <span key={s} className={styles.chip}>
                {s}
              </span>
            ))}
          </div>
        </td>
        <td>
          {appliesTo.length === 0 ? (
            <span className={styles.muted}>—</span>
          ) : (
            <div className={styles.appliesCompact} title={appliesTo.join(', ')}>
              {appliesTo.slice(0, 2).map((k) => (
                <span key={k} className={styles.appliesChip}>
                  {k}
                </span>
              ))}
              {appliesTo.length > 2 && (
                <span className={styles.appliesMore}>+{appliesTo.length - 2}</span>
              )}
            </div>
          )}
        </td>
        <td>
          <div className={styles.chipRow}>
            {sources.map((src) => (
              <span key={src.id} className={styles.sourceChip} title={`priority ${src.priority}`}>
                {src.id}
              </span>
            ))}
          </div>
        </td>
        <td className={styles.numeric}>{rowStats.totalCount.toLocaleString()}</td>
        <td className={styles.muted}>{formatLastSeen(rowStats.lastSeenAt)}</td>
        <td>
          <CoverageBadge state={rowStats.coverage} />
        </td>
      </tr>
      {expanded && (
        <tr className={styles.detailRow} onClick={(e) => e.stopPropagation()}>
          <td colSpan={10}>
            <div className={styles.detail}>
              {metric.description && <p className={styles.description}>{metric.description}</p>}

              {appliesTo.length > 0 && (
                <div className={styles.detailSection}>
                  <div className={styles.detailLabel}>Applies to</div>
                  <div className={styles.chipRow}>
                    {appliesTo.map((k) => (
                      <span key={k} className={styles.appliesChip}>
                        {k}
                      </span>
                    ))}
                  </div>
                </div>
              )}

              <div className={styles.detailSection}>
                <div className={styles.detailLabel}>Sources & samples (last 1h)</div>
                <table className={styles.sourceTable}>
                    <thead>
                      <tr>
                        <th>Source</th>
                        <th>Priority</th>
                        <th>Samples</th>
                        <th>Last seen</th>
                      </tr>
                    </thead>
                    <tbody>
                      {(stats?.by_source ?? sources.map((s) => ({
                        source_id: s.id,
                        priority: s.priority,
                        sample_count: 0,
                        last_seen_at: null,
                      }))).map((s) => (
                        <tr key={s.source_id}>
                          <td>
                            <span className={styles.sourceChip}>{s.source_id}</span>
                          </td>
                          <td className={styles.muted}>{s.priority ?? '—'}</td>
                          <td className={styles.numeric}>{s.sample_count.toLocaleString()}</td>
                          <td className={styles.muted}>
                            {s.last_seen_at
                              ? formatLastSeen(new Date(s.last_seen_at))
                              : '—'}
                          </td>
                        </tr>
                      ))}
                    </tbody>
                </table>
              </div>

              <div className={styles.detailActions}>
                <button
                  type="button"
                  className={styles.openButton}
                  onClick={(e) => {
                    e.stopPropagation()
                    onOpenInExplorer()
                  }}
                >
                  Open in {openMode === 'edge' ? 'Edges' : 'Entities'}
                </button>
              </div>
            </div>
          </td>
        </tr>
      )}
    </>
  )
}
