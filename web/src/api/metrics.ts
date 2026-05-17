import { request } from './client'
import type {
  MetricSamplesResponse,
  MetricSeriesResponse,
  MetricCatalogResponse,
  MetricQueryParams,
  MetricCategory,
  CatalogStatsResponse,
} from '@/types/metrics'

export function getSamples(params: MetricQueryParams): Promise<MetricSamplesResponse> {
  return request<MetricSamplesResponse>('/api/v1/metrics', {
    metric_name: params.metric_name,
    entity_id: params.entity_id,
    edge_source_id: params.edge_source_id,
    edge_target_id: params.edge_target_id,
    from: params.from,
    to: params.to,
    page_size: params.page_size,
    next_token: params.next_token,
  })
}

export function getSeries(params: MetricQueryParams): Promise<MetricSeriesResponse> {
  return request<MetricSeriesResponse>('/api/v1/metrics/series', {
    metric_name: params.metric_name,
    entity_id: params.entity_id,
    edge_source_id: params.edge_source_id,
    edge_target_id: params.edge_target_id,
    from: params.from,
    to: params.to,
    page_size: params.page_size,
    next_token: params.next_token,
  })
}

export function getCatalog(category?: MetricCategory): Promise<MetricCatalogResponse> {
  return request<MetricCatalogResponse>('/api/v1/metrics/catalog', { category })
}

export function getCatalogStats(): Promise<CatalogStatsResponse> {
  return request<CatalogStatsResponse>('/api/v1/metrics/catalog/stats', {})
}
