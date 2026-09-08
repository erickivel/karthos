import { useEffect, useMemo, useRef, useState } from 'react'
import { useSearchParams } from 'react-router-dom'
import { HelpCircle } from 'lucide-react'
import type { AvailableFilters } from '@/types/graph'
import { useAppStore } from '@/stores/useAppStore'
import { useProjectionState } from './useProjectionState'
import { LegendPanel } from './LegendPanel'
import { granularityEntriesFor, MODE_ENTRIES } from './modes'
import { namespaceColor } from './namespaceColor'
import { SearchablePopover } from '@/views/MetricsExplorer/filter/SearchablePopover'
import type { FilterOption } from '@/views/MetricsExplorer/filter/types'
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
    setMode,
    setGranularity,
    setIncludeOrphans,
  } = useProjectionState()
  const granularityEntries = granularityEntriesFor(mode)
  const selectedEntityId = useAppStore((s) => s.selectedEntityId)
  const setSelectedEntityId = useAppStore((s) => s.setSelectedEntityId)
  const [legendOpen, setLegendOpen] = useState(false)

  const currentKind = searchParams.get('kind') ?? ''
  const currentNamespace = searchParams.get('namespace') ?? ''
  const currentRelation = searchParams.get('relation') ?? ''
  const [openPicker, setOpenPicker] = useState<'kind' | 'namespace' | 'relation' | null>(
    null,
  )

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

  const kindOptions = useMemo<FilterOption[]>(
    () => [
      { label: 'All kinds', value: '', meta: 'no kind filter' },
      ...(filters?.kinds ?? []).map((k) => ({ label: k, value: k })),
    ],
    [filters?.kinds],
  )

  const namespaceOptions = useMemo<FilterOption[]>(
    () => [
      { label: 'All namespaces', value: '', meta: 'no namespace filter' },
      ...(filters?.namespaces ?? []).map((ns) => ({ label: ns, value: ns })),
    ],
    [filters?.namespaces],
  )

  const relationOptions = useMemo<FilterOption[]>(
    () => [
      { label: 'All relations', value: '', meta: 'no relation filter' },
      ...(filters?.relations ?? []).map((r) => ({ label: r, value: r })),
    ],
    [filters?.relations],
  )

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
        <div className={styles.breadcrumb}>
          <BreadcrumbSegment
            id="kind"
            label="Kind"
            value={currentKind || 'all kinds'}
            selected={currentKind}
            options={kindOptions}
            picker={openPicker}
            setPicker={setOpenPicker}
            onSelect={(v) => updateFilter('kind', v)}
            testId="filter-kind"
          />
          <BreadcrumbSlash />
          <BreadcrumbSegment
            id="namespace"
            label="Namespace"
            value={currentNamespace || 'all namespaces'}
            selected={currentNamespace}
            options={namespaceOptions}
            picker={openPicker}
            setPicker={setOpenPicker}
            onSelect={(v) => updateFilter('namespace', v)}
            testId="filter-namespace"
          />
          <BreadcrumbSlash />
          <BreadcrumbSegment
            id="relation"
            label="Relation"
            value={currentRelation || 'all relations'}
            selected={currentRelation}
            options={relationOptions}
            picker={openPicker}
            setPicker={setOpenPicker}
            onSelect={(v) => updateFilter('relation', v)}
            testId="filter-relation"
          />
        </div>

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
        <div className={styles.section}>
          <span className={styles.sectionLabel}>View</span>
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
        </div>

        <div className={`${styles.section} ${styles.subSection}`}>
          <span className={styles.sectionLabel}>
            <span className={styles.sectionLabelLead}>Layer</span>
            <span className={styles.sectionLabelMuted}>
              · {MODE_ENTRIES.find((m) => m.value === mode)?.shortLabel ?? mode}
            </span>
          </span>
          <div
            className={styles.group}
            role="group"
            aria-label="Granularity"
            data-testid="topology-granularity-group"
          >
            {granularityEntries.map(({ value, shortLabel, label, icon: Icon, description }) => (
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
        </div>

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

type PickerId = 'kind' | 'namespace' | 'relation' | null

interface BreadcrumbSegmentProps {
  id: Exclude<PickerId, null>
  label: string
  value: string
  selected: string
  options: FilterOption[]
  picker: PickerId
  setPicker: (id: PickerId) => void
  onSelect: (value: string) => void
  testId?: string
}

function BreadcrumbSegment({
  id,
  label,
  value,
  selected,
  options,
  picker,
  setPicker,
  onSelect,
  testId,
}: BreadcrumbSegmentProps) {
  const open = picker === id
  const buttonRef = useRef<HTMLButtonElement>(null)
  return (
    <span className={styles.segmentWrap}>
      <button
        ref={buttonRef}
        type="button"
        className={styles.segment}
        data-open={open}
        data-testid={testId}
        title={`Filter by ${label.toLowerCase()}`}
        aria-label={`Filter by ${label.toLowerCase()}`}
        onClick={() => setPicker(open ? null : id)}
      >
        <span className={styles.segmentLabel}>{label}</span>
        <span className={styles.segmentValue}>{value}</span>
        <BreadcrumbChevron />
      </button>
      <SearchablePopover
        open={open}
        options={options}
        selected={selected}
        onSelect={(v) => {
          onSelect(v)
          setPicker(null)
        }}
        onClose={() => setPicker(null)}
        width={300}
        anchorRef={buttonRef}
      />
    </span>
  )
}

function BreadcrumbSlash() {
  return <span className={styles.slash}>/</span>
}

function BreadcrumbChevron() {
  return (
    <svg
      width={10}
      height={10}
      viewBox="0 0 10 10"
      fill="none"
      stroke="currentColor"
      strokeWidth={1.5}
      aria-hidden
    >
      <path d="M2 4l3 3 3-3" />
    </svg>
  )
}
