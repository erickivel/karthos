import { memo } from 'react'
import { BaseEdge, getSmoothStepPath, type EdgeProps } from '@xyflow/react'
import type { GraphEdge, RelationType } from '@/types/graph'
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

function RelationEdgeComponent(props: EdgeProps) {
  const edge = props.data as unknown as GraphEdge | undefined
  const relation = edge?.relation ?? 'calls'
  const color = getEdgeColor(relation)
  const directed = DIRECTED_RELATIONS.includes(relation)

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
        style={{ stroke: color, strokeWidth: 1.5, opacity: 0.7 }}
        markerEnd={directed ? props.markerEnd : undefined}
      />
      <foreignObject
        x={labelX - 50}
        y={labelY - 10}
        width={100}
        height={20}
        className={styles.labelWrapper}
      >
        <div className={styles.label}>
          {relation}{getDirectionSymbol(relation)}
        </div>
      </foreignObject>
    </>
  )
}

export const RelationEdge = memo(RelationEdgeComponent)
