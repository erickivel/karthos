import { ArrowDownToLine, ArrowUpFromLine, CircleAlert, CircleCheck, RadioTower, ScanSearch, Tags, TriangleAlert, Waypoints } from 'lucide-react'
import { useMemo, useState } from 'react'
import type { CanonicalMetric, MetricQuery, MetricSeries } from '@/types/metrics'
import { InfoTooltip } from './InfoTooltip'
import { summarizeServiceTraffic, type ServiceTrafficDirection, type ServiceTrafficOutcome } from './serviceTrafficModel'
import styles from './ServiceTrafficSummary.module.css'

interface Props {
  metric: CanonicalMetric
  series: MetricSeries[]
  query: MetricQuery
  direction: ServiceTrafficDirection
  serviceName: string
}

const OUTCOME_META: Record<ServiceTrafficOutcome, { label: string; Icon: typeof CircleCheck; className: string }> = {
  successful: { label: 'Successful', Icon: CircleCheck, className: 'success' },
  clientFailures: { label: 'Client failures', Icon: CircleAlert, className: 'failure' },
  serverFailures: { label: 'Server failures', Icon: TriangleAlert, className: 'failure' },
  other: { label: 'Other / unclassified', Icon: CircleAlert, className: 'other' },
}

function destinationKey(source: string, destination: string) {
  return `${source}\u0000${destination}`
}

function formatRate(value: number) {
  return value.toLocaleString(undefined, { maximumFractionDigits: 2 })
}

const DIRECTION_META: Record<ServiceTrafficDirection, {
  heading: string
  rateTitle: string
  meaning: string
  peerPlural: string
  selectedPeer: string
  peerLabel: 'edge_client' | 'edge_server'
  Icon: typeof ArrowDownToLine
}> = {
  inbound: {
    heading: 'Inbound requests',
    rateTitle: 'Inbound request rate',
    meaning: 'Requests other services send to this service per second.',
    peerPlural: 'Callers',
    selectedPeer: 'Selected caller',
    peerLabel: 'edge_client',
    Icon: ArrowDownToLine,
  },
  outbound: {
    heading: 'Outbound requests',
    rateTitle: 'Outbound request rate',
    meaning: 'Requests this service sends to other services per second.',
    peerPlural: 'Destinations',
    selectedPeer: 'Selected destination',
    peerLabel: 'edge_server',
    Icon: ArrowUpFromLine,
  },
}

export function ServiceTrafficSummary({ metric, series, query, direction, serviceName }: Props) {
  const summaries = useMemo(() => summarizeServiceTraffic(series, direction), [direction, series])
  const [selectedKey, setSelectedKey] = useState<string>()
  const directionMeta = DIRECTION_META[direction]
  const DirectionIcon = directionMeta.Icon

  if (summaries.length === 0) return null

  return (
    <section className={styles.summary} aria-label={`${directionMeta.heading} summary`}>
      {summaries.map((summary) => {
        const defaultDestination = summary.destinations[0]
        if (!defaultDestination) return null
        const selected = summary.destinations.find((destination) => destinationKey(summary.source, destination.name) === selectedKey) ?? defaultDestination
        const activeKey = destinationKey(summary.source, selected.name)

        return (
          <section key={summary.source} className={styles.sourceSummary}>
            <div className={styles.metricHeader}>
              <div>
                <span className={styles.eyebrow}><DirectionIcon size={13} aria-hidden="true" /> {directionMeta.heading}</span>
                <div className={styles.titleRow}>
                  <h2>{directionMeta.rateTitle}</h2>
                  <InfoTooltip
                    canonicalName={metric.name}
                    idSuffix={summary.source}
                    sampleCount={series.reduce((count, entry) => count + entry.samples.length, 0)}
                    sources={metric.sources}
                    meaning={directionMeta.meaning}
                    observedSource={summary.source}
                    query={query}
                    seriesLabels={[{ label: 'Observed source', value: summary.source }]}
                  />
                </div>
                <span className={styles.source}><RadioTower size={13} aria-hidden="true" /> Observed source: {summary.source} · complete evaluation: {new Date(summary.timestamp).toLocaleTimeString()}</span>
              </div>
              <strong className={styles.total}>{formatRate(summary.total)} <small>/s</small></strong>
            </div>

            <div className={styles.outcomes}>
              {(Object.keys(OUTCOME_META) as ServiceTrafficOutcome[]).map((outcome) => {
                const { Icon, label, className } = OUTCOME_META[outcome]
                return (
                  <span key={outcome} className={`${styles.outcome} ${styles[className]}`}>
                    <Icon size={14} aria-hidden="true" />
                    {label} <strong>{formatRate(summary.outcomes[outcome])} /s</strong>
                  </span>
                )
              })}
            </div>

            <div className={styles.details}>
              <div className={styles.destinations}>
                <span className={styles.sectionLabel}><Waypoints size={13} aria-hidden="true" /> {directionMeta.peerPlural} · {summary.destinations.length} observed</span>
                <div className={styles.destinationList}>
                  {summary.destinations.map((destination) => {
                    const key = destinationKey(summary.source, destination.name)
                    return (
                      <button
                        key={key}
                        type="button"
                        className={styles.destination}
                        aria-pressed={key === activeKey}
                        onClick={() => setSelectedKey(key)}
                      >
                        <strong>{direction === 'inbound' ? `${destination.name} → ${serviceName}` : `${serviceName} → ${destination.name}`}</strong>
                        <span className={styles.destinationRate}>{formatRate(destination.total)} /s</span>
                        <span className={styles.destinationMeta}>
                          <span className={styles.success}>{formatRate(destination.outcomes.successful)} successful</span>
                          {' · '}
                          <span className={styles.failure}>{formatRate(destination.outcomes.clientFailures + destination.outcomes.serverFailures)} failed</span>
                        </span>
                      </button>
                    )
                  })}
                </div>
              </div>

              <aside className={styles.inspector} aria-live="polite">
                <span className={styles.sectionLabel}><ScanSearch size={13} aria-hidden="true" /> {directionMeta.selectedPeer}</span>
                <div className={styles.inspectorHeading}>
                  <strong>{selected.name}</strong>
                  <span>{formatRate(selected.total)} /s</span>
                </div>
                <p className={styles.inspectorSub}>Exact HTTP outcome series in this evaluation</p>
                <div className={styles.statusList}>
                  {selected.statuses.map((status) => {
                    const meta = OUTCOME_META[status.outcome]
                    return (
                      <div key={status.code} className={styles.status}>
                        <span>HTTP {status.code} · {meta.label}</span>
                        <strong className={styles[meta.className]}>{formatRate(status.value)} /s</strong>
                        <small>http.response.status_code={status.code}</small>
                      </div>
                    )
                  })}
                </div>
                <p className={styles.labels}><Tags size={13} aria-hidden="true" /> <strong>Series labels</strong><br />{directionMeta.peerLabel}={selected.name} · source={summary.source}</p>
              </aside>
            </div>
          </section>
        )
      })}
    </section>
  )
}
