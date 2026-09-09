import type { MetricSample } from '@/types/metrics'
import styles from './RawSamplesTable.module.css'

interface RawSamplesTableProps {
  samples: MetricSample[]
  /**
   * Optional custom scope formatter. Defaults to entity_id or
   * source→target for edge samples. The page passes a snapshot-aware
   * formatter so users see human-readable namespace/name, not raw IDs.
   */
  scopeFor?: (sample: MetricSample) => string
}

const MESH_LABEL_KEYS = [
  'mesh.security.policy',
  'mesh.peer.source.principal',
  'mesh.peer.destination.principal',
  'mesh.envoy.response_flags',
  'mesh.id',
  'network.protocol.name',
  'http.response.status_code',
  'rpc.grpc.status_code',
] as const

function hasMeshMetadata(sample: MetricSample): boolean {
  if (!sample.labels) return false
  return MESH_LABEL_KEYS.some((k) => sample.labels?.[k])
}

function meshSummary(labels: Record<string, string>): { text: string; tooltip: string } | null {
  const parts: string[] = []
  if (labels['mesh.security.policy'] === 'mutual_tls') parts.push('mTLS')
  const protocol = labels['network.protocol.name']
  if (protocol) parts.push(protocol)
  const flags = labels['mesh.envoy.response_flags']
  if (flags && flags !== '-') parts.push(flags)
  if (parts.length === 0) return null

  const tooltipLines: string[] = []
  if (labels['mesh.peer.source.principal']) {
    tooltipLines.push(`source: ${labels['mesh.peer.source.principal']}`)
  }
  if (labels['mesh.peer.destination.principal']) {
    tooltipLines.push(`destination: ${labels['mesh.peer.destination.principal']}`)
  }
  if (labels['http.response.status_code']) {
    tooltipLines.push(`http status: ${labels['http.response.status_code']}`)
  }
  if (labels['rpc.grpc.status_code']) {
    tooltipLines.push(`grpc status: ${labels['rpc.grpc.status_code']}`)
  }

  return { text: parts.join(' · '), tooltip: tooltipLines.join('\n') }
}

function defaultScope(sample: MetricSample): string {
  if (sample.edge) {
    return `${sample.edge.source_id} → ${sample.edge.target_id}`
  }
  return sample.entity_id
}

export function RawSamplesTable({ samples, scopeFor = defaultScope }: RawSamplesTableProps) {
  if (samples.length === 0) return null

  const showMesh = samples.some(hasMeshMetadata)

  return (
    <div className={styles.wrapper}>
      <h3 className={styles.heading}>Raw Samples</h3>
      <div className={styles.tableContainer}>
        <table className={styles.table}>
          <thead>
            <tr>
              <th>Metric</th>
              <th>Scope</th>
              <th>Source</th>
              {showMesh && <th>Mesh</th>}
              <th>Value</th>
              <th>Label</th>
              <th>Unit</th>
              <th>Timestamp</th>
            </tr>
          </thead>
          <tbody>
            {samples.map((s, i) => {
              const source = s.labels?.source
              const mesh = showMesh && s.labels ? meshSummary(s.labels) : null
              return (
                <tr key={i} className={i % 2 === 0 ? styles.rowEven : styles.rowOdd}>
                  <td>{s.metric_name}</td>
                  <td>{scopeFor(s)}</td>
                  <td>{source ? <span className={styles.sourceChip}>{source}</span> : <span className={styles.muted}>-</span>}</td>
                  {showMesh && (
                    <td>
                      {mesh ? (
                        <span className={styles.meshCell} title={mesh.tooltip || undefined}>
                          {mesh.text}
                        </span>
                      ) : (
                        <span className={styles.muted}>-</span>
                      )}
                    </td>
                  )}
                  <td className={styles.value}>{s.value.toFixed(2)}</td>
                  <td>{s.value_label ?? <span className={styles.muted}>-</span>}</td>
                  <td>{s.unit}</td>
                  <td>{new Date(s.timestamp).toLocaleString()}</td>
                </tr>
              )
            })}
          </tbody>
        </table>
      </div>
    </div>
  )
}
