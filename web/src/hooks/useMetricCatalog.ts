import { useQuery } from '@tanstack/react-query'
import { getCatalog } from '@/api/metrics'
import type { MetricCategory } from '@/types/metrics'

export function useMetricCatalog(category?: MetricCategory) {
  return useQuery({
    queryKey: ['metric-catalog', category],
    queryFn: () => getCatalog(category),
    staleTime: Infinity,
  })
}
