import type { EntityKind } from './graph'

export interface Entity {
  id: string
  kind: EntityKind
  cluster: string
  namespace: string
  name: string
  labels: Record<string, string>
}

export interface EntitiesResponse {
  entities: Entity[]
  total: number
}

export interface EntityFilters {
  kind?: EntityKind
  namespace?: string
  page_size?: number
  next_token?: string
}
