import type { MetricSample } from '@/types/metrics'

const NON_SERIES_KEYS = new Set(['source', 'karthos_unit', 'temporality', 'entity_id', 'edge_client_id', 'edge_server_id'])

function sameScope(left: MetricSample, right: MetricSample): boolean {
  if (left.edge || right.edge) {
    return left.edge?.source_id === right.edge?.source_id && left.edge?.target_id === right.edge?.target_id
  }
  return left.entity_id === right.entity_id
}

export function seriesIdentity(samples: MetricSample[], sample: MetricSample): { summary: string; detail: string } {
  const labels = sample.labels ?? {}
  const peers = samples.filter((other) => other.metric_name === sample.metric_name
    && sameScope(other, sample) && other.labels?.source === labels.source)
  const keys = [...new Set(peers.flatMap((other) => Object.keys(other.labels ?? {})))].filter((key) => !NON_SERIES_KEYS.has(key))
  const differing = keys.filter((key) => peers.some((other) => other.labels?.[key] !== labels[key]))
  const visibleKeys = differing.length > 0 ? differing : keys
  return {
    summary: visibleKeys.map((key) => `${key}=${labels[key] ?? '(unset)'}`).join(' · '),
    detail: Object.entries(labels).map(([key, value]) => `${key}=${value}`).join('\n'),
  }
}
