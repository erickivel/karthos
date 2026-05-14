export type EntityKind =
  | 'cluster'
  | 'node'
  | 'namespace'
  | 'workload'
  | 'pod'
  | 'container'
  | 'service'
  | 'ingress'
  | 'endpoint'
  | 'configmap'
  | 'secret'
  | 'pvc'
  | 'hpa'
  | 'resourcequota'

export type RelationType =
  | 'calls'
  | 'runs_on'
  | 'contains'
  | 'routes_to'
  | 'exposes'
  | 'uses'
  | 'mounts'

export interface GraphNode {
  id: string
  kind: EntityKind
  name: string
  namespace: string
  cluster: string
  status: string
  labels?: Record<string, string>
}

export interface GraphEdge {
  id: string
  source: string
  target: string
  relation: RelationType
  confidence: number
  provenance?: string[]
  first_seen: string
  last_seen: string
  metadata?: Record<string, string>
}

export interface Graph {
  timestamp: string
  nodes: GraphNode[]
  edges: GraphEdge[]
}

export interface AvailableFilters {
  kinds: EntityKind[]
  namespaces: string[]
  relations: RelationType[]
}

export interface GraphSummary {
  node_count: number
  edge_count: number
  kind_counts: Record<string, number>
  relation_counts: Record<string, number>
  snapshot_age_seconds: number
  available_filters: AvailableFilters
}

export interface GraphSnapshot {
  version: { major: number; minor: number }
  from: string
  to: string
  created_at: string
  graph: Graph
  summary: GraphSummary
}

export interface EnrichedEdge extends GraphEdge {
  related_node: {
    id: string
    kind: EntityKind
    name: string
    namespace: string
    cluster: string
    labels?: Record<string, string>
  } | null
}

export interface NodeDetail {
  node: GraphNode
  edges: {
    outbound: EnrichedEdge[]
    inbound: EnrichedEdge[]
  }
}

export interface NodesResponse {
  nodes: GraphNode[]
  total: number
}

export interface EdgesResponse {
  edges: GraphEdge[]
  total: number
}

export interface NodeFilters {
  kind?: EntityKind
  namespace?: string
  page_size?: number
  next_token?: string
}

export interface EdgeFilters {
  source?: string
  target?: string
  relation?: RelationType
  page_size?: number
  next_token?: string
}
