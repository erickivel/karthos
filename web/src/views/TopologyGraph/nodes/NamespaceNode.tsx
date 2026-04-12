import { memo } from 'react'
import { type NodeProps } from '@xyflow/react'
import { Boxes } from 'lucide-react'
import type { GraphNode } from '@/types/graph'
import styles from './NamespaceNode.module.css'

interface NamespaceData extends GraphNode {
  nsWidth: number
  nsHeight: number
}

function NamespaceNodeComponent({ data }: NodeProps) {
  const node = data as unknown as NamespaceData
  const width = node.nsWidth ?? 200
  const height = node.nsHeight ?? 80

  return (
    <div className={styles.container} style={{ width, height }}>
      <div className={styles.pill}>
        <Boxes size={12} strokeWidth={1.5} className={styles.pillIcon} />
        <span className={styles.pillLabel}>{node.name}</span>
      </div>
    </div>
  )
}

export const NamespaceNode = memo(NamespaceNodeComponent)
