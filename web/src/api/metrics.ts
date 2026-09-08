import { request } from './client'
import type {
  MetricSeriesResponse,
  MetricCatalogResponse,
  MetricQueryParams,
  MetricCategory,
  CatalogStatsResponse,
} from '@/types/metrics'
import {
  catalogStatsResponseSchema,
  metricCatalogResponseSchema,
  metricSeriesResponseSchema,
} from './schemas'

export function getSeries(params: MetricQueryParams): Promise<MetricSeriesResponse> {
  return request('/api/v1/metrics/series', metricSeriesResponseSchema, {
    metric_name: params.metric_name,
    entity_id: params.entity_id,
    edge_source_id: params.edge_source_id,
    edge_target_id: params.edge_target_id,
    from: params.from,
    to: params.to,
    page_size: params.page_size,
    next_token: params.next_token,
    aggregation: params.aggregation,
    window: params.window,
  }).then(({ data, nextToken }) => ({ ...data, next_token: nextToken }))
}

/**
 * Fetch every cursor page for an internal projection that must not silently
 * treat a partial page as a complete set of dimensional series.
 */
export async function getAllSeries(params: MetricQueryParams): Promise<MetricSeriesResponse> {
  const firstPage = await getSeries(params)
  const series = [...firstPage.series]
  let nextToken = firstPage.next_token

  while (nextToken) {
    const page = await getSeries({ ...params, next_token: nextToken })
    series.push(...page.series)
    nextToken = page.next_token
  }

  return { ...firstPage, series, total: series.length, next_token: undefined }
}

export function getCatalog(category?: MetricCategory): Promise<MetricCatalogResponse> {
  return request('/api/v1/metrics/catalog', metricCatalogResponseSchema, { category }).then(({ data }) => data)
}

export function getCatalogStats(): Promise<CatalogStatsResponse> {
  return request('/api/v1/metrics/catalog/stats', catalogStatsResponseSchema, {}).then(({ data }) => data)
}
