import { useQuery } from '@tanstack/react-query'
import { getSeries } from '@/api/metrics'
import type { MetricQueryParams } from '@/types/metrics'

export function useMetrics(params: MetricQueryParams | null) {
  return useQuery({
    queryKey: ['metrics', params],
    queryFn: () => getSeries(params!),
    enabled: params !== null,
    refetchInterval: 30_000,
  })
}
