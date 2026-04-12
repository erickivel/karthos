import type { MetricSample } from '@/types/metrics'
import styles from './RawSamplesTable.module.css'

interface RawSamplesTableProps {
  samples: MetricSample[]
}

export function RawSamplesTable({ samples }: RawSamplesTableProps) {
  if (samples.length === 0) return null

  return (
    <div className={styles.wrapper}>
      <h3 className={styles.heading}>Raw Samples</h3>
      <div className={styles.tableContainer}>
        <table className={styles.table}>
          <thead>
            <tr>
              <th>Metric</th>
              <th>Value</th>
              <th>Unit</th>
              <th>Timestamp</th>
            </tr>
          </thead>
          <tbody>
            {samples.map((s, i) => (
              <tr key={i} className={i % 2 === 0 ? styles.rowEven : styles.rowOdd}>
                <td>{s.metric_name}</td>
                <td className={styles.value}>{s.value.toFixed(2)}</td>
                <td>{s.unit}</td>
                <td>{new Date(s.timestamp).toLocaleString()}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
    </div>
  )
}
