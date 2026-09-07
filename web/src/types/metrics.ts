import type { z } from 'zod'
import {
  canonicalMetricSchema,
  catalogStatsResponseSchema,
  metricCatalogResponseSchema,
  metricCategorySchema,
  metricSampleSchema,
  metricQuerySchema,
  metricScopeSchema,
  metricSeriesResponseSchema,
  metricSeriesSchema,
} from '@/api/schemas'

export type MetricCategory = z.infer<typeof metricCategorySchema>
export type MetricScope = z.infer<typeof metricScopeSchema>
export type MetricSample = z.infer<typeof metricSampleSchema>
export type MetricQuery = z.infer<typeof metricQuerySchema>
export type MetricSeries = z.infer<typeof metricSeriesSchema>
export type CanonicalMetric = z.infer<typeof canonicalMetricSchema>
export type SourceRef = CanonicalMetric['sources'][number]
export type MetricSeriesResponse = z.infer<typeof metricSeriesResponseSchema>
export type MetricCatalogResponse = z.infer<typeof metricCatalogResponseSchema>
export type CatalogStatsResponse = z.infer<typeof catalogStatsResponseSchema>
export type CatalogStatEntry = CatalogStatsResponse['stats'][number]
export type CatalogStatSource = CatalogStatEntry['by_source'][number]

export interface MetricQueryParams {
  metric_name: string
  entity_id?: string[]
  edge_source_id?: string
  edge_target_id?: string
  from?: string
  to?: string
  aggregation?: 'latest' | 'window_rate' | 'active_rate' | 'increase' | 'window_mean'
  window?: string
  page_size?: number
  next_token?: string
}
