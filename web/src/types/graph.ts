import type { z } from 'zod'
import {
  entityKindSchema,
  graphEdgeSchema,
  graphNodeSchema,
  graphSnapshotSchema,
  relationSchema,
} from '@/api/schemas'

export type EntityKind = z.infer<typeof entityKindSchema>
export type RelationType = z.infer<typeof relationSchema>
export type GraphNode = z.infer<typeof graphNodeSchema>
export type GraphEdge = z.infer<typeof graphEdgeSchema>
export type GraphSnapshot = z.infer<typeof graphSnapshotSchema>
export type Graph = GraphSnapshot['graph']
export type GraphSummary = GraphSnapshot['summary']
export type AvailableFilters = GraphSummary['available_filters']
