import { memo } from 'react'
import { Handle, Position, type NodeProps } from '@xyflow/react'
import {
  Globe,
  Boxes,
  Box,
  Server,
  Container,
  ArrowRightFromLine,
  FileText,
  Lock,
  HardDrive,
} from 'lucide-react'
import type { GraphNode, EntityKind } from '@/types/graph'
import styles from './EntityNode.module.css'

function getHeaderColor(kind: EntityKind, status: string): string {
  if (status === 'error') return 'var(--color-alert)'
  if (['service', 'workload', 'pod'].includes(kind)) return 'var(--color-signal)'
  if (['node', 'namespace'].includes(kind)) return 'var(--color-info)'
  if (kind === 'container') return '#6b7280'
  return 'var(--surface-active)'
}

const SMALL_KINDS: EntityKind[] = ['configmap', 'secret', 'pvc']

const KIND_ICONS: Record<EntityKind, React.ComponentType<{ size?: number; strokeWidth?: number }>> = {
  service: Globe,
  workload: Boxes,
  pod: Box,
  node: Server,
  namespace: Boxes,
  container: Container,
  ingress: ArrowRightFromLine,
  configmap: FileText,
  secret: Lock,
  pvc: HardDrive,
}

interface MetricBarData {
  label: string
  value: string
  ratio: number
}

function formatCpu(cpu: number): string {
  if (cpu >= 1000) return `${(cpu / 1000).toFixed(1)}`
  if (cpu >= 1) return `${cpu.toFixed(1)}m`
  return `${(cpu * 1000).toFixed(0)}µ`
}

function formatMem(mem: number): string {
  if (mem >= 1_073_741_824) return `${(mem / 1_073_741_824).toFixed(1)}G`
  if (mem >= 1_048_576) return `${(mem / 1_048_576).toFixed(0)}M`
  if (mem >= 1024) return `${(mem / 1024).toFixed(0)}K`
  return `${mem.toFixed(0)}B`
}

function getMetricBars(node: GraphNode): MetricBarData[] {
  const bars: MetricBarData[] = []
  const labels = node.labels ?? {}

  const cpuRaw = labels['cpu_usage_millicores']
  if (cpuRaw) {
    const cpu = parseFloat(cpuRaw)
    const maxCpu = parseFloat(labels['cpu_capacity_millicores'] || '100')
    bars.push({
      label: 'CPU',
      value: formatCpu(cpu),
      ratio: Math.min(cpu / maxCpu, 1),
    })
  }

  const memRaw = labels['memory_usage_bytes']
  if (memRaw) {
    const mem = parseFloat(memRaw)
    const maxMem = parseFloat(labels['memory_capacity_bytes'] || String(256 * 1024 * 1024))
    bars.push({
      label: 'MEM',
      value: formatMem(mem),
      ratio: Math.min(mem / maxMem, 1),
    })
  }

  return bars
}

function SegmentedBar({ ratio }: { ratio: number }) {
  const segments = 10
  const filled = Math.round(ratio * segments)
  return (
    <div className={styles.barSegments}>
      {Array.from({ length: segments }, (_, i) => (
        <div
          key={i}
          className={styles.barBlock}
          data-filled={i < filled}
        />
      ))}
    </div>
  )
}

function EntityNodeComponent({ data }: NodeProps) {
  const node = data as unknown as GraphNode
  const isSmall = SMALL_KINDS.includes(node.kind)
  const hasTelemetry = node.status !== 'unknown' || Object.keys(node.labels ?? {}).length > 0
  const IconComponent = KIND_ICONS[node.kind] ?? Box
  const metricBars = getMetricBars(node)

  return (
    <div
      className={styles.card}
      data-small={isSmall}
      data-no-telemetry={!hasTelemetry}
    >
      <Handle type="target" position={Position.Top} className={styles.handle} />

      <div
        className={styles.header}
        style={{ backgroundColor: getHeaderColor(node.kind, node.status) }}
      >
        <span className={styles.name}>{node.name}</span>
        <IconComponent size={14} strokeWidth={1.5} className={styles.icon} />
      </div>

      <div className={styles.body}>
        <div className={styles.prop}>
          <span className={styles.propLabel}>KIND</span>
          <span className={styles.propValue}>{node.kind}</span>
        </div>
        {node.namespace && (
          <div className={styles.prop}>
            <span className={styles.propLabel}>NS</span>
            <span className={styles.propValue}>{node.namespace}</span>
          </div>
        )}
      </div>

      {metricBars.length > 0 && (
        <div className={styles.metrics}>
          {metricBars.map((bar) => (
            <div key={bar.label} className={styles.metricBlock}>
              <div className={styles.metricHeader}>
                <span className={styles.metricLabel}>{bar.label}</span>
                <span className={styles.metricValue}>{bar.value}</span>
              </div>
              <SegmentedBar ratio={bar.ratio} />
            </div>
          ))}
        </div>
      )}

      {!hasTelemetry && (
        <div className={styles.telemetryHint}>Awaiting telemetry data</div>
      )}

      <Handle type="source" position={Position.Bottom} className={styles.handle} />
    </div>
  )
}

export const EntityNode = memo(EntityNodeComponent)
