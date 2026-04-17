import { memo } from 'react'
import { type NodeProps } from '@xyflow/react'
import { Boxes, Server } from 'lucide-react'
import type { EntityKind, GraphNode } from '@/types/graph'
import styles from './NamespaceNode.module.css'

interface NamespaceData extends GraphNode {
  nsWidth: number
  nsHeight: number
  variant?: EntityKind
}

function NamespaceNodeComponent({ data }: NodeProps) {
  const node = data as unknown as NamespaceData
  const width = node.nsWidth ?? 200
  const height = node.nsHeight ?? 80
  const isNode = node.variant === 'node'
  const Icon = isNode ? Server : Boxes
  const containerClass = isNode ? `${styles.container} ${styles.nodeVariant}` : styles.container
  const pillClass = isNode ? `${styles.pill} ${styles.pillNode}` : styles.pill

  return (
    <div className={containerClass} style={{ width, height }}>
      <div className={pillClass}>
        <Icon size={12} strokeWidth={1.5} className={styles.pillIcon} />
        <span className={styles.pillLabel}>{node.name}</span>
      </div>
    </div>
  )
}

export const NamespaceNode = memo(NamespaceNodeComponent)
