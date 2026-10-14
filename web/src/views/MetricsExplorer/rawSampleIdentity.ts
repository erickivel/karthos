import type { MetricSample } from '@/types/metrics'

const NON_SERIES_KEYS = new Set(['source', 'karthos_unit', 'temporality', 'entity_id', 'edge_client_id', 'edge_server_id'])

interface SeriesIdentity {
  summary: string
  detail: string
}

interface Group {
  indexes: number[]
  keys: Map<string, { firstValue: string; differs: boolean; present: number }>
}

function groupKey(sample: MetricSample): string {
  const scope = sample.edge
    ? ['edge', sample.edge.source_id, sample.edge.target_id]
    : ['entity', sample.entity_id]
  return JSON.stringify([sample.metric_name, scope, sample.labels?.source])
}

/** Compute each peer group's distinguishing dimensions once for the full table. */
export function seriesIdentities(samples: MetricSample[]): SeriesIdentity[] {
  const groups = new Map<string, Group>()

  samples.forEach((sample, index) => {
    const key = groupKey(sample)
    let group = groups.get(key)
    if (!group) {
      group = { indexes: [], keys: new Map() }
      groups.set(key, group)
    }
    group.indexes.push(index)

    for (const [label, value] of Object.entries(sample.labels ?? {})) {
      if (NON_SERIES_KEYS.has(label)) continue
      const existing = group.keys.get(label)
      if (existing) {
        existing.present += 1
        if (existing.firstValue !== value) existing.differs = true
      } else {
        group.keys.set(label, { firstValue: value, differs: false, present: 1 })
      }
    }
  })

  const identities: SeriesIdentity[] = new Array(samples.length)
  for (const group of groups.values()) {
    const keys = [...group.keys.entries()]
    const differing = keys.filter(([, state]) => state.differs || state.present !== group.indexes.length)
    const visibleKeys = (differing.length > 0 ? differing : keys).map(([key]) => key)

    for (const index of group.indexes) {
      const labels = samples[index]!.labels ?? {}
      identities[index] = {
        summary: visibleKeys.map((key) => `${key}=${labels[key] ?? '(unset)'}`).join(' · '),
        detail: Object.entries(labels).map(([key, value]) => `${key}=${value}`).join('\n'),
      }
    }
  }
  return identities
}
