import styles from './DataChip.module.css'

interface DataChipProps {
  label: string
  variant?: 'default' | 'signal' | 'info'
}

export function DataChip({ label, variant = 'default' }: DataChipProps) {
  return (
    <span className={styles.chip} data-variant={variant}>
      {label}
    </span>
  )
}
