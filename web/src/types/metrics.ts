export type MetricCategory =
  | 'resource'
  | 'traffic'
  | 'latency'
  | 'error'
  | 'capacity'

export interface MetricSample {
  metric_name: string
  entity_id: string
  value: number
  timestamp: string
  unit: string
}

export interface MetricSeries {
  metric_name: string
  unit: string
  category: MetricCategory
  description: string
  samples: MetricSample[]
}

export interface CanonicalMetric {
  name: string
  unit: string
  category: MetricCategory
  description: string
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
  from?: string
  to?: string
  page_size?: number
  next_token?: string
}
