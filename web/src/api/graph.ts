import { request } from './client'
import type {
  GraphSnapshot,
  NodeDetail,
  NodesResponse,
  EdgesResponse,
  NodeFilters,
  EdgeFilters,
} from '@/types/graph'

export function getSnapshot(): Promise<GraphSnapshot> {
  return request<GraphSnapshot>('/api/v1/graph')
}

export function getNodes(filters?: NodeFilters): Promise<NodesResponse> {
  return request<NodesResponse>('/api/v1/graph/nodes', filters as Record<string, string | undefined>)
}

export function getNode(id: string): Promise<NodeDetail> {
  return request<NodeDetail>(`/api/v1/graph/nodes/${encodeURIComponent(id)}`)
}

export function getEdges(filters?: EdgeFilters): Promise<EdgesResponse> {
  return request<EdgesResponse>('/api/v1/graph/edges', filters as Record<string, string | undefined>)
}
