import { useQuery } from '@tanstack/react-query'
import { listEntities } from '@/api/entities'
import type { EntityFilters } from '@/types/entities'

export function useEntities(filters?: EntityFilters) {
  return useQuery({
    queryKey: ['entities', filters],
    queryFn: () => listEntities(filters),
  })
}
