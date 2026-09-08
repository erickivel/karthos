import type { CatalogStatEntry } from '@/types/metrics'

export type CoverageState = 'live' | 'stale' | 'no-data'

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
export function computeRowStats(
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

  let coverage: CoverageState
  if (totalCount === 0 && !lastSeenAt) {
    coverage = 'no-data'
  } else if (lastSeenAt && nowMs - lastSeenAt.getTime() <= LIVE_THRESHOLD_MS) {
    coverage = 'live'
  } else {
    coverage = 'stale'
  }
  return { totalCount, lastSeenAt, coverage }
}
