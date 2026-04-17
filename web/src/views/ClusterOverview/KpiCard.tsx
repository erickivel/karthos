import styles from './KpiCard.module.css'

interface KpiCardProps {
  label: string
  value: number
  accent?: 'signal' | 'alert' | 'info' | 'muted'
}

export function KpiCard({ label, value, accent = 'signal' }: KpiCardProps) {
  return (
    <div className={styles.card}>
      <span className={styles.value} data-accent={accent}>
        {value.toLocaleString()}
      </span>
      <span className={styles.label}>{label}</span>
    </div>
  )
}
