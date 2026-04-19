import { useEntities } from '@/hooks/useEntities'
import styles from './MetricsExplorer.module.css'

interface EntitySelectorProps {
  value: string
  onChange: (id: string) => void
}

export function EntitySelector({ value, onChange }: EntitySelectorProps) {
  const { data } = useEntities({ page_size: 1000 })

  return (
    <select
      className={styles.select}
      value={value}
      onChange={(e) => onChange(e.target.value)}
      aria-label="Select entity"
    >
      <option value="">Select entity...</option>
      {data?.entities.map((entity) => (
        <option key={entity.id} value={entity.id}>
          [{entity.kind}] {entity.namespace ? `${entity.namespace}/` : ''}{entity.name}
        </option>
      ))}
    </select>
  )
}
