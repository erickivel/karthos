import { useEffect, useMemo, useRef, useState, type CSSProperties, type RefObject } from 'react'
import { useMetricCatalog } from '@/hooks/useMetricCatalog'
import { useMetricCatalogStats } from '@/hooks/useMetricCatalogStats'
import { useClickOutside } from '@/hooks/useClickOutside'
import type { CanonicalMetric, CatalogStatEntry, MetricScope } from '@/types/metrics'
import type { FilterOption } from '../filter/types'
import { MetricCatalogRow } from './MetricCatalogRow'
import { computeRowStats } from './coverage'
import {
  EMPTY_CATALOG_FILTERS,
  filterCatalogRows,
  sortCatalogRows,
  type CatalogFilterState,
  type CatalogSortDir,
  type CatalogSortKey,
  type CatalogTableRowModel,
} from './tableModel'
import styles from './MetricCatalogTable.module.css'

type FilterPickerId = Exclude<keyof CatalogFilterState, 'query'> | null

interface MetricCatalogTableProps {
  onOpenInExplorer: (metric: CanonicalMetric, mode: 'entity' | 'edge') => void
}

function pickOpenMode(scopes: MetricScope[] | undefined): 'entity' | 'edge' {
  if (scopes && scopes.includes('entity')) return 'entity'
  if (scopes && scopes.includes('edge')) return 'edge'
  return 'entity'
}

export function MetricCatalogTable({ onOpenInExplorer }: MetricCatalogTableProps) {
  const { data: catalog, isLoading: catalogLoading } = useMetricCatalog()
  const { data: stats, isLoading: statsLoading } = useMetricCatalogStats()
  const [expanded, setExpanded] = useState<Set<string>>(new Set())
  const [sortKey, setSortKey] = useState<CatalogSortKey>('category')
  const [sortDir, setSortDir] = useState<CatalogSortDir>('asc')
  const [filters, setFilters] = useState<CatalogFilterState>(EMPTY_CATALOG_FILTERS)
  const [picker, setPicker] = useState<FilterPickerId>(null)

  const rows = useMemo<CatalogTableRowModel[]>(() => {
    if (!catalog?.metrics) return []
    const statByName = new Map<string, CatalogStatEntry>()
    for (const s of stats?.stats ?? []) statByName.set(s.metric_name, s)
    const now = Date.now()
    return catalog.metrics.map((metric) => {
      const entry = statByName.get(metric.name)
      return { metric, stats: entry, rowStats: computeRowStats(entry, now) }
    })
  }, [catalog?.metrics, stats?.stats])

  const filterOptions = useMemo(() => deriveFilterOptions(rows, filters), [filters, rows])

  const filteredRows = useMemo(
    () => filterCatalogRows(rows, filters),
    [filters, rows],
  )

  const sortedRows = useMemo(() => {
    return sortCatalogRows(filteredRows, sortKey, sortDir)
  }, [filteredRows, sortKey, sortDir])

  const handleSort = (key: CatalogSortKey) => {
    if (sortKey === key) {
      setSortDir(sortDir === 'asc' ? 'desc' : 'asc')
    } else {
      setSortKey(key)
      setSortDir(key === 'samples' || key === 'last_seen' ? 'desc' : 'asc')
    }
  }

  const sortIndicator = (key: CatalogSortKey) => (sortKey === key ? (sortDir === 'asc' ? ' ▲' : ' ▼') : '')

  const toggleRow = (name: string) => {
    setExpanded((prev) => {
      const next = new Set(prev)
      if (next.has(name)) next.delete(name)
      else next.add(name)
      return next
    })
  }

  if (catalogLoading || statsLoading) {
    return <div className={styles.empty}>Loading catalog…</div>
  }
  if (!catalog?.metrics?.length) {
    return <div className={styles.empty}>No catalog metrics defined.</div>
  }

  const patchQuery = (value: string) => {
    setFilters((prev) => ({ ...prev, query: value }))
  }
  const patchFilter = (key: FilterPickerId, value: string[]) => {
    if (!key) return
    setFilters((prev) => ({ ...prev, [key]: value }))
  }
  const activeFilterCount = [
    filters.query ? 1 : 0,
    filters.source.length,
    filters.category.length,
    filters.unit.length,
    filters.scope.length,
    filters.coverage.length,
  ].reduce((sum, count) => sum + count, 0)

  return (
    <div className={styles.wrapper}>
      <div className={styles.summary}>
        <span>
          <strong>{sortedRows.length}</strong> of <strong>{rows.length}</strong> canonical metrics
        </span>
        <span className={styles.muted}>
          Coverage window: last {Math.round((stats?.lookback_seconds ?? 3600) / 60)} minutes
        </span>
      </div>
      <div className={styles.toolbar}>
        <label className={styles.searchBar}>
          <SearchIcon />
          <input
            value={filters.query}
            onChange={(e) => patchQuery(e.target.value)}
            placeholder="Search metrics by name, canonical id, source, unit, or description…"
            className={styles.searchInput}
            aria-label="Search catalog metrics"
          />
        </label>
        <div className={styles.filterRow} aria-label="Catalog metric filters">
          <CatalogFilterPicker
            id="source"
            label="Source"
            value={filters.source}
            options={filterOptions.source}
            picker={picker}
            setPicker={setPicker}
            onChange={(value) => patchFilter('source', value)}
          />
          <CatalogFilterPicker
            id="category"
            label="Category"
            value={filters.category}
            options={filterOptions.category}
            picker={picker}
            setPicker={setPicker}
            onChange={(value) => patchFilter('category', value)}
          />
          <CatalogFilterPicker
            id="unit"
            label="Unit"
            value={filters.unit}
            options={filterOptions.unit}
            picker={picker}
            setPicker={setPicker}
            onChange={(value) => patchFilter('unit', value)}
          />
          <CatalogFilterPicker
            id="scope"
            label="Scope"
            value={filters.scope}
            options={filterOptions.scope}
            picker={picker}
            setPicker={setPicker}
            onChange={(value) => patchFilter('scope', value)}
          />
          <CatalogFilterPicker
            id="coverage"
            label="Coverage"
            value={filters.coverage}
            options={filterOptions.coverage}
            picker={picker}
            setPicker={setPicker}
            onChange={(value) => patchFilter('coverage', value)}
          />
          {activeFilterCount > 0 && (
            <button
              type="button"
              className={styles.clearButton}
              onClick={() => {
                setFilters(EMPTY_CATALOG_FILTERS)
                setPicker(null)
              }}
            >
              Clear
            </button>
          )}
        </div>
      </div>
      <div className={styles.tableContainer}>
        <table className={styles.table}>
          <thead>
            <tr>
              <th aria-label="expand" />
              <th>
                <button type="button" className={styles.sortBtn} onClick={() => handleSort('name')}>
                  Name{sortIndicator('name')}
                </button>
              </th>
              <th>
                <button type="button" className={styles.sortBtn} onClick={() => handleSort('category')}>
                  Category{sortIndicator('category')}
                </button>
              </th>
              <th>Unit</th>
              <th>Scopes</th>
              <th>Applies to</th>
              <th>
                <button type="button" className={styles.sortBtn} onClick={() => handleSort('sources')}>
                  Sources{sortIndicator('sources')}
                </button>
              </th>
              <th className={styles.numericHead}>
                <button type="button" className={styles.sortBtn} onClick={() => handleSort('samples')}>
                  Samples (1h){sortIndicator('samples')}
                </button>
              </th>
              <th>
                <button type="button" className={styles.sortBtn} onClick={() => handleSort('last_seen')}>
                  Last seen{sortIndicator('last_seen')}
                </button>
              </th>
              <th>Coverage</th>
            </tr>
          </thead>
          <tbody>
            {sortedRows.map(({ metric, stats: entry, rowStats }) => {
              const openMode = pickOpenMode(metric.scopes)
              return (
                <MetricCatalogRow
                  key={metric.name}
                  metric={metric}
                  stats={entry}
                  rowStats={rowStats}
                  expanded={expanded.has(metric.name)}
                  openMode={openMode}
                  onToggle={() => toggleRow(metric.name)}
                  onOpenInExplorer={() => onOpenInExplorer(metric, openMode)}
                />
              )
            })}
            {sortedRows.length === 0 && (
              <tr>
                <td colSpan={10} className={styles.noResults}>
                  No metrics match the current filters.
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </div>
  )
}

function deriveFilterOptions(rows: CatalogTableRowModel[], filters: CatalogFilterState) {
  const sourceRows = rowsForFacet(rows, filters, 'source')
  const categoryRows = rowsForFacet(rows, filters, 'category')
  const unitRows = rowsForFacet(rows, filters, 'unit')
  const scopeRows = rowsForFacet(rows, filters, 'scope')
  const coverageRows = rowsForFacet(rows, filters, 'coverage')

  return {
    source: mapOptions(countSources(sourceRows), 'source', sourceRows.length, filters.source),
    category: mapOptions(countField(categoryRows, (row) => [row.metric.category]), 'category', categoryRows.length, filters.category),
    unit: mapOptions(countField(unitRows, (row) => (row.metric.unit ? [row.metric.unit] : [])), 'unit', unitRows.length, filters.unit),
    scope: mapOptions(countField(scopeRows, (row) => row.metric.scopes ?? []), 'scope', scopeRows.length, filters.scope),
    coverage: mapOptions(countField(coverageRows, (row) => [row.rowStats.coverage]), 'coverage', coverageRows.length, filters.coverage),
  }
}

function rowsForFacet(
  rows: CatalogTableRowModel[],
  filters: CatalogFilterState,
  facet: Exclude<keyof CatalogFilterState, 'query'>,
) {
  return filterCatalogRows(rows, { ...filters, [facet]: [] })
}

function countSources(rows: CatalogTableRowModel[]): Map<string, number> {
  const sourceCounts = new Map<string, number>()
  for (const row of rows) {
    for (const source of row.metric.sources) {
      sourceCounts.set(source.id, (sourceCounts.get(source.id) ?? 0) + 1)
    }
  }
  return sourceCounts
}

function countField(
  rows: CatalogTableRowModel[],
  valuesForRow: (row: CatalogTableRowModel) => readonly string[],
): Map<string, number> {
  const counts = new Map<string, number>()
  for (const row of rows) {
    for (const value of valuesForRow(row)) counts.set(value, (counts.get(value) ?? 0) + 1)
  }
  return counts
}

function mapOptions(values: Map<string, number>, label: string, totalCount: number, selected: string[]): FilterOption[] {
  const entries = new Map(values)
  for (const value of selected) {
    if (value && !entries.has(value)) entries.set(value, 0)
  }
  return [
    { label: `Any ${label}`, value: '', meta: `${totalCount} metrics` },
    ...Array.from(entries.entries())
      .sort(([a], [b]) => a.localeCompare(b))
      .map(([value, count]) => ({
        label: value,
        value,
        meta: `${count} metric${count === 1 ? '' : 's'}`,
      })),
  ]
}

interface CatalogFilterPickerProps {
  id: Exclude<keyof CatalogFilterState, 'query'>
  label: string
  value: string[]
  options: FilterOption[]
  picker: FilterPickerId
  setPicker: (id: FilterPickerId) => void
  onChange: (value: string[]) => void
}

function CatalogFilterPicker({
  id,
  label,
  value,
  options,
  picker,
  setPicker,
  onChange,
}: CatalogFilterPickerProps) {
  const open = picker === id
  const buttonRef = useRef<HTMLButtonElement>(null)
  const selected = options.filter((option) => option.value && value.includes(option.value))
  const displayValue = selectedLabel(label, selected)
  return (
    <span className={styles.filterWrap}>
      <button
        ref={buttonRef}
        type="button"
        className={styles.filterButton}
        data-open={open}
        data-active={value.length > 0}
        onClick={() => setPicker(open ? null : id)}
        aria-label={`Filter catalog by ${label.toLowerCase()}`}
      >
        <span className={styles.filterLabel}>{label}</span>
        <span className={styles.filterValue}>{displayValue}</span>
        <Chevron />
      </button>
      <CatalogMultiSelectPopover
        open={open}
        options={options}
        selected={value}
        onChange={onChange}
        onClose={() => setPicker(null)}
        width={320}
        anchorRef={buttonRef}
      />
    </span>
  )
}

function selectedLabel(label: string, selected: FilterOption[]): string {
  if (selected.length === 0) return `Any ${label.toLowerCase()}`
  if (selected.length === 1) return selected[0]?.label ?? `Any ${label.toLowerCase()}`
  return `${selected.length} selected`
}

interface CatalogMultiSelectPopoverProps {
  open: boolean
  options: FilterOption[]
  selected: string[]
  onChange: (value: string[]) => void
  onClose: () => void
  width?: number
  anchorRef?: RefObject<HTMLElement | null>
}

function CatalogMultiSelectPopover({
  open,
  options,
  selected,
  onChange,
  onClose,
  width = 320,
  anchorRef,
}: CatalogMultiSelectPopoverProps) {
  const [q, setQ] = useState('')
  const [align, setAlign] = useState<'left' | 'right'>('left')
  const inputRef = useRef<HTMLInputElement>(null)
  const popoverRef = useRef<HTMLDivElement>(null)
  const safeRefs = useMemo(
    () => (anchorRef ? [popoverRef, anchorRef] : [popoverRef]),
    [anchorRef],
  )
  useClickOutside(safeRefs, open, onClose)

  const filtered = useMemo(() => {
    const query = q.trim().toLowerCase()
    const selectable = options.filter((option) => option.value)
    if (!query) return selectable
    return selectable.filter((option) =>
      `${option.label} ${option.meta ?? ''} ${option.value}`.toLowerCase().includes(query),
    )
  }, [options, q])

  useEffect(() => {
    if (!open) return
    setQ('')
    const anchor = anchorRef?.current
    if (anchor) {
      const rect = anchor.getBoundingClientRect()
      const viewportPadding = 16
      const wouldOverflowRight = rect.left + width > window.innerWidth - viewportPadding
      const wouldOverflowLeft = rect.right - width < viewportPadding
      setAlign(wouldOverflowRight && !wouldOverflowLeft ? 'right' : 'left')
    }
    const id = window.setTimeout(() => inputRef.current?.focus(), 0)
    return () => window.clearTimeout(id)
  }, [anchorRef, open, width])

  if (!open) return null

  const selectedSet = new Set(selected)
  const toggle = (value: string) => {
    if (selectedSet.has(value)) onChange(selected.filter((item) => item !== value))
    else onChange([...selected, value])
  }
  const popoverStyle: CSSProperties = align === 'right'
    ? { width, right: 0 }
    : { width, left: 0 }

  return (
    <div
      ref={popoverRef}
      className={styles.multiPopover}
      style={popoverStyle}
      role="listbox"
      aria-multiselectable="true"
    >
      <div className={styles.popoverSearchRow}>
        <SearchIcon />
        <input
          ref={inputRef}
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'Escape') {
              e.preventDefault()
              onClose()
            }
          }}
          placeholder="Filter options…"
          className={styles.popoverInput}
          aria-label="Filter options"
        />
      </div>
      <div className={styles.popoverActions}>
        <button type="button" className={styles.popoverAction} onClick={() => onChange([])}>
          Clear
        </button>
        <span className={styles.selectedCount}>{selected.length} selected</span>
      </div>
      <div className={styles.popoverList}>
        {filtered.length === 0 && <div className={styles.popoverEmpty}>No options</div>}
        {filtered.map((option) => {
          const checked = selectedSet.has(option.value)
          return (
            <button
              key={`${option.value}|${option.label}`}
              type="button"
              onMouseDown={(ev) => ev.preventDefault()}
              onClick={() => toggle(option.value)}
              className={styles.multiOption}
              data-active={checked}
              role="option"
              aria-selected={checked}
            >
              <span className={styles.checkBox} data-checked={checked}>
                {checked ? <CheckIcon /> : null}
              </span>
              <span className={styles.optionLabel}>{option.label}</span>
              {option.meta && <span className={styles.optionMeta}>{option.meta}</span>}
            </button>
          )
        })}
      </div>
    </div>
  )
}

function CheckIcon() {
  return (
    <svg width={12} height={12} viewBox="0 0 12 12" aria-hidden>
      <path d="M2.5 6.2 5 8.5 9.5 3.5" fill="none" stroke="currentColor" strokeWidth={1.6} strokeLinecap="round" strokeLinejoin="round" />
    </svg>
  )
}

function SearchIcon() {
  return (
    <svg width={16} height={16} viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth={1.5} aria-hidden>
      <circle cx="7" cy="7" r="4.5" />
      <path d="M10.5 10.5 14 14" />
    </svg>
  )
}

function Chevron() {
  return (
    <svg width={12} height={12} viewBox="0 0 12 12" aria-hidden>
      <path d="M3 4.5 6 7.5 9 4.5" fill="none" stroke="currentColor" strokeWidth={1.4} strokeLinecap="round" />
    </svg>
  )
}
