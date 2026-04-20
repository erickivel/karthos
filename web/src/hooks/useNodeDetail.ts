import { useQuery } from '@tanstack/react-query'
import { getNode } from '@/api/graph'

export function useNodeDetail(nodeId: string | null) {
  return useQuery({
    queryKey: ['node-detail', nodeId],
    queryFn: () => getNode(nodeId!),
    enabled: nodeId !== null,
  })
}
