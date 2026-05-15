import type { GraphNode } from '@/types/graph'
import { entityKindLabel } from '@/utils/entityDisplay'
import { TimeRangeSelector, type TimeRange } from './TimeRangeSelector'
import { nodeHealth } from './filter/types'
import styles from './SelectionSummary.module.css'

interface EntitySummaryProps {
  kind: 'entity'
  node: GraphNode
  timeRange: TimeRange
  onTimeRangeChange: (range: TimeRange) => void
}

interface EdgeSummaryProps {
  kind: 'edge'
  source: GraphNode | null
  target: GraphNode | null
  relation: string
  timeRange: TimeRange
  onTimeRangeChange: (range: TimeRange) => void
}

type Props = EntitySummaryProps | EdgeSummaryProps

export function SelectionSummary(props: Props) {
  return (
    <div className={styles.card}>
      {props.kind === 'entity' ? <EntityBlock node={props.node} /> : <EdgeBlock {...props} />}
      <TimeRangeSelector value={props.timeRange} onChange={props.onTimeRangeChange} />
    </div>
  )
}

function EntityBlock({ node }: { node: GraphNode }) {
  const health = nodeHealth(node.status)
  return (
    <div className={styles.block}>
      <span className={styles.eyebrow}>Selected entity</span>
      <div className={styles.titleRow}>
        <span className={styles.healthDot} data-health={health} />
        <span className={styles.title}>{node.name}</span>
        <KindBadge kind={node.kind} label={entityKindLabel(node)} />
      </div>
      <div className={styles.chipRow}>
        {node.namespace && <Chip label="ns" value={node.namespace} />}
        {node.cluster && <Chip label="cluster" value={node.cluster} />}
        {node.status && <Chip label="status" value={node.status} />}
        {node.labels?.['app.kubernetes.io/instance'] && (
          <Chip label="app" value={node.labels['app.kubernetes.io/instance']} />
        )}
      </div>
    </div>
  )
}

function EdgeBlock({ source, target, relation }: Omit<EdgeSummaryProps, 'kind' | 'timeRange' | 'onTimeRangeChange'>) {
  return (
    <div className={styles.block}>
      <span className={styles.eyebrow}>Selected edge</span>
      <div className={styles.titleRow}>
        <span className={styles.healthDot} data-health={source ? nodeHealth(source.status) : 'unknown'} />
        <span className={styles.title}>
          {source ? source.name : 'unknown source'}
        </span>
        <span className={styles.arrow}>→</span>
        <span className={styles.title}>
          {target ? target.name : 'unknown target'}
        </span>
        <span className={styles.relationBadge}>{relation}</span>
      </div>
      <div className={styles.chipRow}>
        {source?.namespace && <Chip label="src ns" value={source.namespace} />}
        {target?.namespace && <Chip label="tgt ns" value={target.namespace} />}
        {source && <Chip label="src kind" value={source.kind} />}
        {target && <Chip label="tgt kind" value={target.kind} />}
      </div>
    </div>
  )
}

function KindBadge({ kind, label }: { kind: string; label: string }) {
  return (
    <span className={styles.kindBadge}>
      <span className={styles.kindSwatch} style={{ background: `var(--kind-${kind}, var(--color-text-muted))` }} />
      {label}
    </span>
  )
}

function Chip({ label, value }: { label: string; value: string }) {
  return (
    <span className={styles.chip}>
      <span className={styles.chipLabel}>{label}</span>
      <span className={styles.chipValue}>{value}</span>
    </span>
  )
}
