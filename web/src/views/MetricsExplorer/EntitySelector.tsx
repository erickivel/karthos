import styles from './MetricsExplorer.module.css'
import type { EntityOption } from './model'

interface EntitySelectorProps {
  value: string
  onChange: (id: string) => void
  options: EntityOption[]
}

export function EntitySelector({ value, onChange, options }: EntitySelectorProps) {
  return (
    <select
      className={styles.select}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      aria-label="Select entity"
    >
      <option value="">Select entity...</option>
      {options.map((entity) => (
        <option key={entity.id} value={entity.id}>
          {entity.label}
        </option>
      ))}
    </select>
  )
}
