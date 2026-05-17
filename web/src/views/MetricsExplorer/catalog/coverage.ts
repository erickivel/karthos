import type { CanonicalMetric, CatalogStatEntry } from '@/types/metrics'

export type CoverageState = 'live' | 'stale' | 'no-data' | 'derived'

// LIVE_THRESHOLD_MS gates 'live' vs 'stale'. 5 minutes lines up with the
// catalogStatsLookback floor where a healthy scrape is expected to land at
// least one datapoint per pipeline tick.
const LIVE_THRESHOLD_MS = 5 * 60 * 1000

export interface CatalogRowStats {
  totalCount: number
  lastSeenAt: Date | null
  coverage: CoverageState
}

// computeRowStats folds the per-source breakdown into a single row summary.
// 'derived' takes priority over 'no-data' for catalog entries that have no
// declared Sources (query-composed at fetch time, e.g. health status); we
// don't want those to show up as red 'no data' even though no raw samples
// exist for the canonical name.
export function computeRowStats(
  metric: CanonicalMetric,
  entry: CatalogStatEntry | undefined,
  nowMs: number,
): CatalogRowStats {
  let totalCount = 0
  let lastSeenAt: Date | null = null
  if (entry?.by_source) {
    for (const s of entry.by_source) {
      totalCount += s.sample_count
      if (s.last_seen_at) {
        const ts = new Date(s.last_seen_at)
        if (!lastSeenAt || ts > lastSeenAt) lastSeenAt = ts
      }
    }
  }

  const isDerived = !metric.sources || metric.sources.length === 0
  let coverage: CoverageState
  if (totalCount === 0 && !lastSeenAt) {
    coverage = isDerived ? 'derived' : 'no-data'
  } else if (lastSeenAt && nowMs - lastSeenAt.getTime() <= LIVE_THRESHOLD_MS) {
    coverage = 'live'
  } else {
    coverage = 'stale'
  }
  return { totalCount, lastSeenAt, coverage }
}
