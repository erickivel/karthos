import { request } from './client'
import type { GraphSnapshot } from '@/types/graph'
import { graphSnapshotSchema } from './schemas'

export function getSnapshot(): Promise<GraphSnapshot> {
  return request('/api/v1/graph', graphSnapshotSchema).then(({ data }) => data)
}
