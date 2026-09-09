import type { CoverageState } from './coverage'
import styles from './CoverageBadge.module.css'

const LABELS: Record<CoverageState, string> = {
  live: 'live',
  stale: 'stale',
  'no-data': 'no data',
}

const TITLES: Record<CoverageState, string> = {
  live: 'Samples observed in the last 5 minutes',
  stale: 'Samples in the lookback window but none in the last 5 minutes',
  'no-data': 'No samples in the lookback window — the metric is advertised but not flowing',
}

export function CoverageBadge({ state }: { state: CoverageState }) {
  return (
    <span className={styles.badge} data-state={state} title={TITLES[state]}>
      <span className={styles.dot} aria-hidden />
      {LABELS[state]}
    </span>
  )
}
