import styles from './MetricLoadingSkeleton.module.css'

interface Props {
  count?: number
  fullWidth?: boolean
  label: string
}

/** A stable visual frame while metric-series requests and chart data arrive. */
export function MetricLoadingSkeleton({ count = 2, fullWidth = false, label }: Props) {
  return (
    <div className={`${styles.grid} ${fullWidth ? styles.fullWidth : ''}`} role="status" aria-label={label}>
      {Array.from({ length: count }, (_, index) => (
        <div key={index} className={styles.card} aria-hidden="true">
          <span className={styles.lineShort} />
          <span className={styles.lineLong} />
          <span className={styles.chart} />
        </div>
      ))}
    </div>
  )
}
