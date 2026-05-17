import { useQuery } from '@tanstack/react-query'
import { getCatalogStats } from '@/api/metrics'

// Polls /api/v1/metrics/catalog/stats every 30s; matches the cadence used by
// the series fetch on the Entities/Edges tabs so the coverage badges age in
// lockstep with chart data when both views are open.
export function useMetricCatalogStats() {
  return useQuery({
    queryKey: ['metric-catalog-stats'],
    queryFn: getCatalogStats,
    staleTime: 30_000,
    refetchInterval: 30_000,
  })
}
