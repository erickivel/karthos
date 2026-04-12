import { request } from './client'
import type { Entity, EntitiesResponse, EntityFilters } from '@/types/entities'

export function listEntities(filters?: EntityFilters): Promise<EntitiesResponse> {
  return request<EntitiesResponse>('/api/v1/entities', filters as Record<string, string | undefined>)
}

export function getEntity(id: string): Promise<Entity> {
  return request<Entity>(`/api/v1/entities/${encodeURIComponent(id)}`)
}
