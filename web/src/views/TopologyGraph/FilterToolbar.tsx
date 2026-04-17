import { useSearchParams } from 'react-router-dom'
import type { AvailableFilters } from '@/types/graph'
import {
  GRANULARITIES,
  PROJECTION_MODES,
  type Granularity,
  type ProjectionMode,
} from './projection'
import { useProjectionState } from './useProjectionState'
import styles from './FilterToolbar.module.css'

interface FilterToolbarProps {
  filters: AvailableFilters | undefined
}

const MODE_LABEL: Record<ProjectionMode, string> = {
  ownership: 'Ownership',
  infrastructure: 'Infra',
  traffic: 'Traffic',
}

const GRANULARITY_LABEL: Record<Granularity, string> = {
  overview: 'L1',
  runtime: 'L2',
  detailed: 'L3',
}

export function FilterToolbar({ filters }: FilterToolbarProps) {
  const [searchParams, setSearchParams] = useSearchParams()
  const { mode, granularity, setMode, setGranularity } = useProjectionState()

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
      <div className={styles.group} role="group" aria-label="Projection mode">
        {PROJECTION_MODES.map((m) => (
          <button
            key={m}
            type="button"
            className={`${styles.toggle} ${mode === m ? styles.toggleActive : ''}`}
            aria-pressed={mode === m}
            onClick={() => setMode(m)}
            data-testid={`mode-${m}`}
          >
            {MODE_LABEL[m]}
          </button>
        ))}
      </div>

      <div className={styles.group} role="group" aria-label="Granularity">
        {GRANULARITIES.map((g) => (
          <button
            key={g}
            type="button"
            className={`${styles.toggle} ${granularity === g ? styles.toggleActive : ''}`}
            aria-pressed={granularity === g}
            onClick={() => setGranularity(g)}
            data-testid={`granularity-${g}`}
          >
            {GRANULARITY_LABEL[g]}
          </button>
        ))}
      </div>

      <select
        className={styles.select}
        value={currentKind}
        onChange={(e) => updateFilter('kind', e.target.value)}
      >
        <option value="">All Kinds</option>
        {filters?.kinds.map((k) => (
          <option key={k} value={k}>
            {k}
          </option>
        ))}
      </select>

      <select
        className={styles.select}
        value={currentNamespace}
        onChange={(e) => updateFilter('namespace', e.target.value)}
      >
        <option value="">All Namespaces</option>
        {filters?.namespaces.map((ns) => (
          <option key={ns} value={ns}>
            {ns}
          </option>
        ))}
      </select>

      <select
        className={styles.select}
        value={currentRelation}
        onChange={(e) => updateFilter('relation', e.target.value)}
      >
        <option value="">All Relations</option>
        {filters?.relations.map((r) => (
          <option key={r} value={r}>
            {r}
          </option>
        ))}
      </select>
    </div>
  )
}
