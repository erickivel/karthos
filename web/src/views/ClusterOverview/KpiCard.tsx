import styles from './KpiCard.module.css'

interface KpiCardProps {
  label: string
  value: number
}

export function KpiCard({ label, value }: KpiCardProps) {
  return (
    <div className={styles.card}>
      <span className={styles.value}>{value.toLocaleString()}</span>
      <span className={styles.label}>{label}</span>
    </div>
  )
}
