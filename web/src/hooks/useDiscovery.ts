import { useQuery } from '@tanstack/react-query'
import { getDiscovery } from '@/api/discovery'

export function useDiscovery() {
  return useQuery({
    queryKey: ['discovery'],
    queryFn: getDiscovery,
    refetchInterval: 120_000,
  })
}
