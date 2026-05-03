import { useEffect, useMemo, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { HelpCircle } from 'lucide-react'
import type { AvailableFilters } from '@/types/graph'
import { useAppStore } from '@/stores/useAppStore'
import { useProjectionState } from './useProjectionState'
import { LegendPanel } from './LegendPanel'
import { GRANULARITY_ENTRIES, MODE_ENTRIES } from './modes'
import { namespaceColor } from './namespaceColor'
import { availableAlgorithms, LAYOUT_REGISTRY, type LayoutAlgorithm } from './layouts/registry'
import styles from './FilterToolbar.module.css'

interface FilterToolbarProps {
  filters: AvailableFilters | undefined
}

export function FilterToolbar({ filters }: FilterToolbarProps) {
  const [searchParams, setSearchParams] = useSearchParams()
  const {
    mode,
    granularity,
    includeOrphans,
    layoutAlgorithm,
    setMode,
    setGranularity,
    setIncludeOrphans,
    setLayoutAlgorithm,
  } = useProjectionState()
  const modeAlgorithms = availableAlgorithms(mode)
  const activeAlgorithm = layoutAlgorithm ?? LAYOUT_REGISTRY[mode].defaultAlgorithm
  const selectedEntityId = useAppStore((s) => s.selectedEntityId)
  const setSelectedEntityId = useAppStore((s) => s.setSelectedEntityId)
  const [legendOpen, setLegendOpen] = useState(false)

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

  const legendNamespaces = useMemo(() => {
    if (mode !== 'infrastructure') return []
    return (filters?.namespaces ?? []).slice().sort()
  }, [mode, filters?.namespaces])

  useEffect(() => {
    if (selectedEntityId) {
      setLegendOpen(false)
    }
  }, [selectedEntityId])

  const toggleLegend = () => {
    setLegendOpen((open) => {
      if (!open) {
        setSelectedEntityId(null)
      }
      return !open
    })
  }

  return (
    <>
      <div className={styles.toolbar}>
        <select
          className={styles.select}
          value={currentKind}
          onChange={(e) => updateFilter('kind', e.target.value)}
          data-testid="filter-kind"
          title="Filter by entity kind"
          aria-label="Filter by entity kind"
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
          data-testid="filter-namespace"
          title="Filter by namespace"
          aria-label="Filter by namespace"
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
          data-testid="filter-relation"
          title="Filter by relation type"
          aria-label="Filter by relation type"
        >
          <option value="">All Relations</option>
          {filters?.relations.map((r) => (
            <option key={r} value={r}>
              {r}
            </option>
          ))}
        </select>

        {legendNamespaces.length > 0 && (
          <div className={styles.legend} data-testid="namespace-legend">
            <span className={styles.legendTitle}>Namespaces</span>
            {legendNamespaces.map((ns) => (
              <span key={ns} className={styles.legendItem}>
                <span className={styles.legendSwatch} style={{ background: namespaceColor(ns) }} />
                {ns}
              </span>
            ))}
          </div>
        )}
      </div>

      <div
        className={styles.toolbarRight}
        data-inspector-open={Boolean(selectedEntityId || legendOpen)}
      >
        <div
          className={styles.group}
          role="group"
          aria-label="Projection mode"
          data-testid="topology-mode-group"
        >
          {MODE_ENTRIES.map(({ value, shortLabel, label, icon: Icon, description }) => (
            <button
              key={value}
              type="button"
              className={`${styles.toggle} ${mode === value ? styles.toggleActive : ''}`}
              aria-pressed={mode === value}
              title={`${label} — ${description}`}
              onClick={() => setMode(value)}
              data-testid={`mode-${value}`}
            >
              <Icon size={12} strokeWidth={1.75} className={styles.toggleIcon} />
              <span>{shortLabel}</span>
            </button>
          ))}
        </div>

        <div
          className={styles.group}
          role="group"
          aria-label="Granularity"
          data-testid="topology-granularity-group"
        >
          {GRANULARITY_ENTRIES.map(({ value, shortLabel, label, icon: Icon, description }) => (
            <button
              key={value}
              type="button"
              className={`${styles.toggle} ${granularity === value ? styles.toggleActive : ''}`}
              aria-pressed={granularity === value}
              title={`${label} — ${description}`}
              onClick={() => setGranularity(value)}
              data-testid={`granularity-${value}`}
            >
              <Icon size={12} strokeWidth={1.75} className={styles.toggleIcon} />
              <span>{shortLabel}</span>
            </button>
          ))}
        </div>

        {modeAlgorithms.length > 1 && (
          <select
            className={styles.select}
            value={activeAlgorithm}
            onChange={(e) => {
              const next = e.target.value as LayoutAlgorithm
              setLayoutAlgorithm(
                next === LAYOUT_REGISTRY[mode].defaultAlgorithm ? undefined : next,
              )
            }}
            data-testid="topology-algorithm-select"
            title="Layout algorithm"
            aria-label="Layout algorithm"
          >
            {modeAlgorithms.map((algo) => (
              <option key={algo} value={algo}>
                {algo}
              </option>
            ))}
          </select>
        )}

        <label
          className={styles.toggleCheckbox}
          title="Include entities with no layer-relevant edges"
          data-testid="topology-orphans-toggle"
        >
          <input
            type="checkbox"
            checked={includeOrphans}
            onChange={(e) => setIncludeOrphans(e.target.checked)}
          />
          <span>Show orphans</span>
        </label>

        <button
          type="button"
          className={`${styles.help} ${legendOpen ? styles.helpActive : ''}`}
          onClick={toggleLegend}
          aria-pressed={legendOpen}
          aria-label="Toggle topology legend"
          title="Show legend: modes, granularity and entity colors"
          data-testid="topology-legend-toggle"
        >
          <HelpCircle size={14} strokeWidth={1.75} />
          <span>Legend</span>
        </button>
      </div>

      {legendOpen && <LegendPanel onClose={() => setLegendOpen(false)} />}
    </>
  )
}
