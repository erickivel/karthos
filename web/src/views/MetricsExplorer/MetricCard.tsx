import {
  BarChart,
  Bar,
  CartesianGrid,
  XAxis,
  YAxis,
  ResponsiveContainer,
  Tooltip,
} from 'recharts'
import type { MetricQuery, MetricSample, MetricScope, SourceRef } from '@/types/metrics'
import { formatMetricValue } from '../../utils/formatters'
import { InfoTooltip } from './InfoTooltip'
import { metricPresentation } from './metricPresentation'
import styles from './MetricCard.module.css'

interface MetricCardProps {
  name: string
  canonicalName: string
  unit: string
  samples: MetricSample[]
  sources?: SourceRef[]
  query: MetricQuery
  description: string
  scope: Extract<MetricScope, 'entity' | 'edge'>
  labels?: Record<string, string>
  edge?: { source_id: string; target_id: string }
  emptyMessage?: string
  /** Recharts syncId — cards sharing this value share tooltip + cursor. */
  syncId?: string
}

interface TooltipPayloadEntry {
  value: number
  payload: { time: string; fullTime: string; value: number }
}

function CustomTooltip({
  active,
  payload,
  unit,
}: {
  active?: boolean
  payload?: TooltipPayloadEntry[]
  unit: string
}) {
  if (!active || !payload || payload.length === 0) return null
  const entry = payload[0]!
  const display = formatMetricValue(entry.value, unit)
  return (
    <div className={styles.tooltip}>
      <span className={styles.tooltipValue}>
        {display.value} {display.unit}
      </span>
      <span className={styles.tooltipTime}>{entry.payload.fullTime}</span>
    </div>
  )
}

export function MetricCard({
  name,
  canonicalName,
  unit,
  samples,
  sources,
  query,
  description,
  scope,
  labels,
  edge,
  emptyMessage,
  syncId,
}: MetricCardProps) {
  const sorted = [...samples].sort(
    (a, b) => new Date(a.timestamp).getTime() - new Date(b.timestamp).getTime(),
  )

  const hasSamples = sorted.length > 0
  const currentValue = hasSamples ? sorted[sorted.length - 1]!.value : 0
  const activeSource = hasSamples ? sorted[sorted.length - 1]!.labels?.source : undefined
  const valueDisplay = hasSamples ? formatMetricValue(currentValue, unit) : undefined
  const presentation = metricPresentation({
    canonicalName,
    scope,
    labels,
    fallbackMeaning: description,
    fallbackTitle: name,
  })

  const sortedSources = sources ? [...sources].sort((a, b) => a.priority - b.priority) : []

  const chartData = sorted.map((s, i) => ({
    time: new Date(s.timestamp).toLocaleTimeString([], { hour: '2-digit', minute: '2-digit' }),
    fullTime: new Date(s.timestamp).toLocaleTimeString([], {
      hour: '2-digit',
      minute: '2-digit',
      second: '2-digit',
    }),
    value: s.value,
    index: i,
  }))

  // Show 5 evenly-spaced ticks on the time axis (left, three middles, right)
  // and rewrite the rightmost label to "now" for orientation.
  const tickIndices = chartData.length > 4
    ? [0, Math.floor(chartData.length / 4), Math.floor(chartData.length / 2), Math.floor((3 * chartData.length) / 4), chartData.length - 1]
    : chartData.map((_, i) => i)
  const ticks = tickIndices.map((i) => chartData[i]?.time).filter(Boolean) as string[]
  const tickFormatter = (val: string, i: number) => (i === ticks.length - 1 ? 'now' : val)

  return (
    <div className={styles.card} data-empty={!hasSamples}>
      <div className={styles.header}>
        <div className={styles.headingCol}>
          <div className={styles.titleRow}>
            <span className={styles.name}>{presentation.title}</span>
            <InfoTooltip
              canonicalName={canonicalName}
              sampleCount={sorted.length}
              sources={sortedSources}
              meaning={presentation.meaning}
              observedSource={activeSource}
              query={query}
              seriesLabels={presentation.seriesLabels}
            />
          </div>
          {sortedSources.length > 0 && (
            <div className={styles.sourceRow}>
              {sortedSources.map((s) => (
                <span
                  key={s.id}
                  className={styles.sourceChip}
                  title={`declared priority ${s.priority}`}
                >
                  {s.id}
                </span>
              ))}
            </div>
          )}
        </div>
        <span className={styles.value}>
          {valueDisplay?.value ?? '—'}
          {valueDisplay?.unit && <span className={styles.unit}> {valueDisplay.unit}</span>}
        </span>
      </div>

      <div className={styles.queryMeta}>
        <span>{query.aggregation}{query.window ? ` · ${query.window}` : ''}</span>
        <span>{query.step} grid · {query.timestamp_semantics}</span>
        <span>{new Date(query.from).toLocaleTimeString()} → {new Date(query.to).toLocaleTimeString()}</span>
        {activeSource && <span>sample source: {activeSource}</span>}
        {edge && <span>edge: {edge.source_id} → {edge.target_id}</span>}
      </div>

      {hasSamples ? (
        <div className={styles.chart}>
          <ResponsiveContainer width="100%" height={140}>
            <BarChart data={chartData} barCategoryGap="20%" syncId={syncId} margin={{ top: 8, right: 4, bottom: 0, left: 0 }}>
              <CartesianGrid
                stroke="var(--surface-divider)"
                strokeOpacity={0.35}
                vertical={false}
              />
              <XAxis
                dataKey="time"
                ticks={ticks}
                tickFormatter={tickFormatter}
                interval={0}
                tick={{ fontSize: 10, fill: 'var(--color-text-muted)', fontFamily: 'ui-monospace, monospace' }}
                axisLine={{ stroke: 'var(--surface-divider)' }}
                tickLine={false}
              />
              <YAxis hide domain={[0, 'dataMax']} />
              <Tooltip
                content={<CustomTooltip unit={unit} />}
                cursor={{ fill: 'rgba(255, 255, 255, 0.05)' }}
              />
              <Bar dataKey="value" fill="var(--color-chart-bar, var(--color-signal))" radius={[2, 2, 0, 0]} />
            </BarChart>
          </ResponsiveContainer>
        </div>
      ) : (
        <div className={styles.empty}>
          <span className={styles.emptyMessage}>{emptyMessage ?? 'No samples in selected time range'}</span>
          {sortedSources.length > 0 && (
            <span className={styles.emptyHint}>
              Expected from: {sortedSources.map((s) => s.id).join(', ')}
            </span>
          )}
        </div>
      )}
    </div>
  )
}
