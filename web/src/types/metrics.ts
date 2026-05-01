export type MetricCategory =
  | 'resource'
  | 'traffic'
  | 'latency'
  | 'error'
  | 'capacity'
  | 'health'

export type MetricScope = 'entity' | 'edge'

export interface MetricEdgeRef {
  source_id: string
  target_id: string
}

export interface MetricSample {
  /** OTel semantic-convention metric identifier in dot notation. */
  metric_name: string
  entity_id: string
  value: number
  timestamp: string
  unit: string
  labels?: Record<string, string>
  edge?: MetricEdgeRef
}

export interface MetricSeries {
  metric_name: string
  unit: string
  category: MetricCategory
  description: string
  samples: MetricSample[]
  edge?: MetricEdgeRef
}

export interface SourceRef {
  id: string
  priority: number
}

export type SelectionPolicy = 'priority' | 'disjoint' | 'all'

export interface CanonicalMetric {
  name: string
  display_name: string
  unit: string
  category: MetricCategory
  scopes: MetricScope[]
  description: string
  sources?: SourceRef[]
  selection_policy?: SelectionPolicy
}

export interface MetricSamplesResponse {
  samples: MetricSample[]
  total: number
}

export interface MetricSeriesResponse {
  series: MetricSeries[]
  total: number
}

export interface MetricCatalogResponse {
  metrics: CanonicalMetric[]
  total: number
}

export interface MetricQueryParams {
  metric_name: string
  entity_id?: string[]
  edge_source_id?: string
  edge_target_id?: string
  from?: string
  to?: string
  page_size?: number
  next_token?: string
}
