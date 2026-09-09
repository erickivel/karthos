import type { MetricSeries } from '@/types/metrics'
import { latestSample } from '@/utils/metricSeries'

export type ServiceTrafficOutcome = 'successful' | 'clientFailures' | 'serverFailures' | 'other'
export type ServiceTrafficDirection = 'inbound' | 'outbound'

export interface ServiceTrafficOutcomes {
  successful: number
  clientFailures: number
  serverFailures: number
  other: number
}

export interface ServiceTrafficStatus {
  code: string
  outcome: ServiceTrafficOutcome
  value: number
}

export interface ServiceTrafficDestination {
  name: string
  total: number
  outcomes: ServiceTrafficOutcomes
  statuses: ServiceTrafficStatus[]
}

export interface ServiceTrafficSummary {
  source: string
  timestamp: string
  total: number
  outcomes: ServiceTrafficOutcomes
  destinations: ServiceTrafficDestination[]
}

function emptyOutcomes(): ServiceTrafficOutcomes {
  return { successful: 0, clientFailures: 0, serverFailures: 0, other: 0 }
}

function currentSample(entry: MetricSeries) {
  return latestSample(entry.samples)
}

function outcomeForStatus(code: string): ServiceTrafficOutcome {
  const numeric = Number.parseInt(code, 10)
  if (numeric >= 200 && numeric < 300) return 'successful'
  if (numeric >= 400 && numeric < 500) return 'clientFailures'
  if (numeric >= 500 && numeric < 600) return 'serverFailures'
  return 'other'
}

function latestTimestamp(series: MetricSeries[]): string | undefined {
  const timestamps = series.map((entry) => currentSample(entry)?.timestamp)
  if (timestamps.some((timestamp) => !timestamp)) return undefined
  return timestamps.every((timestamp) => timestamp === timestamps[0]) ? timestamps[0] : undefined
}

/**
 * Creates one aggregate per observed source. A source aggregate is emitted only
 * when every dimensional series shares its newest evaluation timestamp, so the
 * display never mixes a current value with a stale one.
 */
export function summarizeServiceTraffic(
  series: MetricSeries[],
  direction: ServiceTrafficDirection = 'outbound',
): ServiceTrafficSummary[] {
  const peerLabel = direction === 'inbound' ? 'edge_client' : 'edge_server'
  const bySource = new Map<string, MetricSeries[]>()
  for (const entry of series) {
    const source = currentSample(entry)?.labels?.source ?? 'unknown'
    const group = bySource.get(source) ?? []
    group.push(entry)
    bySource.set(source, group)
  }

  const summaries: ServiceTrafficSummary[] = []
  for (const [source, sourceSeries] of bySource) {
    const timestamp = latestTimestamp(sourceSeries)
    if (!timestamp) continue

    const destinations = new Map<string, ServiceTrafficDestination>()
    const outcomes = emptyOutcomes()
    let total = 0

    for (const entry of sourceSeries) {
      const sample = currentSample(entry)
      if (!sample || sample.timestamp !== timestamp) continue
      const labels = sample.labels ?? {}
      const destinationName = labels[peerLabel] ?? `Unattributed ${direction === 'inbound' ? 'caller' : 'destination'}`
      const statusCode = labels['http.response.status_code'] ?? labels.http_status_code ?? 'Unclassified'
      const outcome = outcomeForStatus(statusCode)
      const destination: ServiceTrafficDestination = destinations.get(destinationName) ?? {
        name: destinationName,
        total: 0,
        outcomes: emptyOutcomes(),
        statuses: [],
      }

      total += sample.value
      outcomes[outcome] += sample.value
      destination.total += sample.value
      destination.outcomes[outcome] += sample.value

      const existingStatus = destination.statuses.find((status) => status.code === statusCode)
      if (existingStatus) {
        existingStatus.value += sample.value
      } else {
        destination.statuses.push({ code: statusCode, outcome, value: sample.value })
      }
      destinations.set(destinationName, destination)
    }

    summaries.push({
      source,
      timestamp,
      total,
      outcomes,
      destinations: [...destinations.values()]
        .map((destination) => ({
          ...destination,
          statuses: [...destination.statuses].sort((left, right) => left.code.localeCompare(right.code, undefined, { numeric: true })),
        }))
        .sort((left, right) => right.total - left.total || left.name.localeCompare(right.name)),
    })
  }

  return summaries.sort((left, right) => left.source.localeCompare(right.source))
}
