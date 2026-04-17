import { memo } from 'react'
import { BaseEdge, getSmoothStepPath, type EdgeProps } from '@xyflow/react'
import type { GraphEdge, RelationType } from '@/types/graph'
import type { GraphEdgeMetricData } from '../edgeMetrics'
import styles from './RelationEdge.module.css'

function getEdgeColor(relation: RelationType): string {
  switch (relation) {
    case 'calls':
    case 'exposes':
      return 'var(--color-signal)'
    case 'runs_on':
    case 'contains':
      return 'var(--color-info)'
    default:
      return 'var(--color-text-muted)'
  }
}

// Relations where direction is meaningful and should show an arrow
const DIRECTED_RELATIONS: RelationType[] = ['calls', 'runs_on', 'routes_to', 'exposes']

function getDirectionSymbol(relation: RelationType): string {
  if (DIRECTED_RELATIONS.includes(relation)) return ' \u2192'
  return ''
}

function clamp(value: number, min: number, max: number): number {
  return Math.min(Math.max(value, min), max)
}

function formatLatency(ns?: number): string | null {
  if (ns === undefined) return null
  const ms = ns / 1_000_000
  if (ms >= 1000) return `${(ms / 1000).toFixed(1)}s`
  if (ms >= 100) return `${Math.round(ms)}ms`
  if (ms >= 10) return `${ms.toFixed(1)}ms`
  return `${ms.toFixed(2)}ms`
}

function getMetricState(edge: GraphEdge & GraphEdgeMetricData) {
  const rate = edge.metrics?.requestRate ?? 0
  const latencyNs = edge.metrics?.latencyNs
  const errorRatio = edge.metrics?.errorRatio ?? 0
  const errorRate = edge.metrics?.errorRate ?? 0

  const width = 1.5 + clamp(Math.log10(rate + 1) * 2.2, 0, 5)
  const opacity = 0.38 + clamp(Math.log10(rate + 1) * 0.18, 0, 0.45)
  const latencyMs = latencyNs !== undefined ? latencyNs / 1_000_000 : 0

  let stroke = getEdgeColor(edge.relation)
  if (errorRatio >= 0.1 || errorRate >= 1) {
    stroke = 'var(--color-alert)'
  } else if (latencyMs >= 300) {
    stroke = '#d97706'
  } else if (latencyMs >= 120) {
    stroke = 'var(--color-info)'
  }

  return {
    width,
    opacity: clamp(opacity, 0.38, 0.88),
    stroke,
    glow: clamp(width * 1.35, 3, 9),
    summary: [
      rate > 0 ? `${rate.toFixed(rate >= 10 ? 0 : 1)} rps` : null,
      formatLatency(latencyNs),
      errorRatio > 0 ? `${(errorRatio * 100).toFixed(1)}% err` : null,
    ]
      .filter(Boolean)
      .join(' · '),
  }
}

function RelationEdgeComponent(props: EdgeProps) {
  const edge = props.data as unknown as (GraphEdge & GraphEdgeMetricData) | undefined
  const relation = edge?.relation ?? 'calls'
  const directed = DIRECTED_RELATIONS.includes(relation)
  const metricState = edge ? getMetricState(edge) : null

  const [edgePath, labelX, labelY] = getSmoothStepPath({
    sourceX: props.sourceX,
    sourceY: props.sourceY,
    targetX: props.targetX,
    targetY: props.targetY,
    sourcePosition: props.sourcePosition,
    targetPosition: props.targetPosition,
    borderRadius: 8,
  })

  return (
    <>
      <BaseEdge
        id={props.id}
        path={edgePath}
        style={{
          stroke: metricState?.stroke ?? getEdgeColor(relation),
          strokeWidth: metricState?.width ?? 1.5,
          opacity: metricState?.opacity ?? 0.7,
          filter: metricState ? `drop-shadow(0 0 ${metricState.glow}px ${metricState.stroke})` : undefined,
        }}
        markerEnd={directed ? props.markerEnd : undefined}
      />
      <foreignObject
        x={labelX - 68}
        y={labelY - 18}
        width={136}
        height={36}
        className={styles.labelWrapper}
      >
        <div className={styles.label}>
          <span>
            {relation}
            {getDirectionSymbol(relation)}
          </span>
          {metricState?.summary && (
            <span className={styles.metricSummary}>{metricState.summary}</span>
          )}
        </div>
      </foreignObject>
    </>
  )
}

export const RelationEdge = memo(RelationEdgeComponent)
