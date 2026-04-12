import { useSearchParams } from 'react-router-dom'
import type { AvailableFilters } from '@/types/graph'
import styles from './FilterToolbar.module.css'

interface FilterToolbarProps {
  filters: AvailableFilters | undefined
}

export function FilterToolbar({ filters }: FilterToolbarProps) {
  const [searchParams, setSearchParams] = useSearchParams()

  const currentKind = searchParams.get('kind') ?? ''
  const currentNamespace = searchParams.get('namespace') ?? ''
  const currentRelation = searchParams.get('relation') ?? ''

  const updateFilter = (key: string, value: string) => {
    setSearchParams((prev) => {
      const next = new URLSearchParams(prev)
      if (value) {
        next.set(key, value)
      } else {
        next.delete(key)
      }
      return next
    })
  }

  return (
    <div className={styles.toolbar}>
      <select
        className={styles.select}
        value={currentKind}
        onChange={(e) => updateFilter('kind', e.target.value)}
      >
        <option value="">All Kinds</option>
        {filters?.kinds.map((k) => (
          <option key={k} value={k}>{k}</option>
        ))}
      </select>

      <select
        className={styles.select}
        value={currentNamespace}
        onChange={(e) => updateFilter('namespace', e.target.value)}
      >
        <option value="">All Namespaces</option>
        {filters?.namespaces.map((ns) => (
          <option key={ns} value={ns}>{ns}</option>
        ))}
      </select>

      <select
        className={styles.select}
        value={currentRelation}
        onChange={(e) => updateFilter('relation', e.target.value)}
      >
        <option value="">All Relations</option>
        {filters?.relations.map((r) => (
          <option key={r} value={r}>{r}</option>
        ))}
      </select>
    </div>
  )
}
