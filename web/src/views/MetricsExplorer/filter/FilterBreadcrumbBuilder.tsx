import { useEffect, useMemo, useRef, useState } from 'react'
import type { EntityKind, GraphNode } from '@/types/graph'
import { entityKindLabel } from '@/utils/entityDisplay'
import { SearchablePopover } from './SearchablePopover'
import {
  applyFilters,
  compareNodes,
  nodeHealth,
  nodeLabel,
  type FilterOption,
  type FilterState,
} from './types'
import styles from './FilterBreadcrumbBuilder.module.css'

interface Props {
  nodes: GraphNode[] | undefined
  state: FilterState
  setState: (next: FilterState) => void
}

const KIND_COLOR: Record<EntityKind, string> = {
  cluster: 'var(--kind-cluster)',
  node: 'var(--kind-node)',
  namespace: 'var(--kind-namespace)',
  workload: 'var(--kind-workload)',
  pod: 'var(--kind-pod)',
  container: 'var(--kind-container)',
  service: 'var(--kind-service)',
  ingress: 'var(--kind-ingress)',
  endpoint: 'var(--kind-endpoint)',
  configmap: 'var(--kind-configmap)',
  secret: 'var(--kind-secret)',
  pvc: 'var(--kind-pvc)',
  hpa: 'var(--kind-workload)',
}

type PickerId = 'namespace' | 'kind' | 'entity' | 'status' | null

export function FilterBreadcrumbBuilder({ nodes, state, setState }: Props) {
  const [editing, setEditing] = useState(false)
  const [picker, setPicker] = useState<PickerId>(null)

  const allNodes = useMemo(() => nodes ?? [], [nodes])
  const candidates = useMemo(() => applyFilters(allNodes, state), [allNodes, state])
  // scopedEntities now respects every filter except `entityId` itself, so the
  // entity picker only lists candidates that match the current namespace,
  // kind, status, and query.
  const scopedEntities = useMemo(
    () => applyFilters(allNodes, { ...state, entityId: '' }),
    [allNodes, state],
  )

  const namespaceOptions = useMemo<FilterOption[]>(() => {
    // Narrow by every other field so the user never sees a namespace that
    // would yield zero matches under their other filters.
    const scoped = applyFilters(allNodes, {
      ...state,
      namespace: '',
      entityId: '',
    })
    const counts = new Map<string, number>()
    for (const n of scoped) {
      if (!n.namespace) continue
      counts.set(n.namespace, (counts.get(n.namespace) ?? 0) + 1)
    }
    const sorted = [...counts.entries()].sort((a, b) => a[0].localeCompare(b[0]))
    return [
      { label: 'All namespaces', value: '', meta: `${scoped.length} entities` },
      ...sorted.map(([ns, n]) => ({ label: ns, value: ns, meta: `${n} entities` })),
    ]
  }, [allNodes, state])

  const kindOptions = useMemo<FilterOption[]>(() => {
    const scoped = applyFilters(allNodes, { ...state, kind: '', entityId: '' })
    const counts = new Map<EntityKind, number>()
    for (const n of scoped) counts.set(n.kind, (counts.get(n.kind) ?? 0) + 1)
    const sorted = [...counts.entries()].sort((a, b) => a[0].localeCompare(b[0]))
    return [
      { label: 'Any kind', value: '', meta: 'all kinds' },
      ...sorted.map(([kind, n]) => ({
        label: kind,
        value: kind,
        meta: `${n}`,
        color: KIND_COLOR[kind],
      })),
    ]
  }, [allNodes, state])

  const entityOptions = useMemo<FilterOption[]>(() => {
    const sorted = [...scopedEntities].sort(compareNodes).slice(0, 300)
    return [
      {
        label: `Pick from ${scopedEntities.length} scoped entities`,
        value: '',
        meta: 'no entity selected',
      },
      ...sorted.map((n) => ({
        label: n.name,
        value: n.id,
        meta: `${n.namespace || 'cluster'} / ${entityKindLabel(n)}`,
        health: nodeHealth(n.status),
        color: KIND_COLOR[n.kind],
      })),
    ]
  }, [scopedEntities])

  const statusOptions: FilterOption[] = [
    { label: 'any', value: '', meta: 'all health states' },
    { label: 'healthy', value: 'healthy', health: 'healthy' },
    { label: 'running, not ready', value: 'running_not_ready', health: 'running_not_ready' },
    { label: 'restarting', value: 'restarting', health: 'restarting' },
    { label: 'pending', value: 'pending', health: 'pending' },
    { label: 'succeeded', value: 'succeeded', health: 'succeeded' },
    { label: 'failed', value: 'failed', health: 'failed' },
    { label: 'unknown', value: 'unknown', health: 'unknown' },
  ]

  const selectedNode = allNodes.find((n) => n.id === state.entityId)

  // If the currently selected entity no longer satisfies the other filters
  // (user changed kind/status/query/namespace and made it ineligible),
  // clear it so the breadcrumbs don't lie about the active selection.
  useEffect(() => {
    if (!state.entityId) return
    const stillMatches = scopedEntities.some((n) => n.id === state.entityId)
    if (!stillMatches) {
      setState({ ...state, entityId: '' })
    }
  }, [scopedEntities, state, setState])

  const activeAdvanced = state.status || state.query
  const hasAny = state.namespace || state.kind || state.entityId || activeAdvanced

  const patch = (edits: Partial<FilterState>) => setState({ ...state, ...edits })

  const reset = () => setState({ namespace: '', kind: '', entityId: '', query: '', status: '' })

  return (
    <div className={styles.root}>
      <div className={styles.header}>
        <div className={styles.path}>
          <PathSegment
            id="namespace"
            label="Namespace"
            value={state.namespace || 'all namespaces'}
            options={namespaceOptions}
            selected={state.namespace}
            picker={picker}
            setPicker={setPicker}
            onSelect={(v) => patch({ namespace: v, kind: '', entityId: '' })}
          />
          <Slash />
          <PathSegment
            id="kind"
            label="Kind"
            value={state.kind || 'any kind'}
            color={state.kind ? KIND_COLOR[state.kind as EntityKind] : undefined}
            options={kindOptions}
            selected={state.kind}
            picker={picker}
            setPicker={setPicker}
            onSelect={(v) => patch({ kind: v as EntityKind | '', entityId: '' })}
          />
          <Slash />
          <PathSegment
            id="entity"
            label="Entity"
            value={selectedNode ? selectedNode.name : state.kind ? `select ${state.kind}` : 'any entity'}
            options={entityOptions}
            selected={state.entityId}
            picker={picker}
            setPicker={setPicker}
            width={360}
            onSelect={(v) => {
              const node = allNodes.find((n) => n.id === v)
              if (!node) {
                patch({ entityId: '' })
                return
              }
              patch({
                entityId: v,
                namespace: node.namespace || '',
                kind: node.kind,
                query: '',
              })
            }}
          />
          {activeAdvanced ? (
            <span className={styles.activeFlag}>+ filters applied</span>
          ) : null}
        </div>

        <div className={styles.actions}>
          <span className={styles.matches}>
            {candidates.length} match{candidates.length === 1 ? '' : 'es'}
          </span>
          <button
            type="button"
            className={styles.editButton}
            data-active={editing}
            onClick={() => setEditing((v) => !v)}
          >
            <FilterIcon />
            {editing ? 'Collapse' : 'Edit Filters'}
          </button>
          {hasAny ? (
            <button type="button" className={styles.resetButton} onClick={reset}>
              Reset
            </button>
          ) : null}
        </div>
      </div>

      {editing && (
        <div className={styles.builder}>
          <BuilderRow keyword="SCOPE">
            <BuilderCombobox
              id="builder-namespace"
              label="namespace"
              value={state.namespace}
              options={namespaceOptions}
              picker={picker}
              setPicker={setPicker}
              onChange={(v) => patch({ namespace: v, entityId: '' })}
            />
            <BuilderCombobox
              id="builder-kind"
              label="kind"
              value={state.kind}
              options={kindOptions}
              picker={picker}
              setPicker={setPicker}
              onChange={(v) => patch({ kind: v as EntityKind | '', entityId: '' })}
            />
          </BuilderRow>

          <BuilderRow keyword="WHERE">
            <BuilderCombobox
              id="builder-status"
              label="status"
              value={state.status}
              options={statusOptions}
              picker={picker}
              setPicker={setPicker}
              onChange={(v) => patch({ status: v, entityId: '' })}
            />
            <label className={styles.queryWrap}>
              <span className={styles.queryLabel}>name contains</span>
              <input
                className={styles.queryInput}
                value={state.query}
                placeholder="text…"
                onChange={(e) => patch({ query: e.target.value, entityId: '' })}
              />
            </label>
          </BuilderRow>

          <BuilderRow keyword="SELECT">
            <BuilderCombobox
              id="builder-entity"
              label="entity"
              value={state.entityId}
              options={entityOptions}
              picker={picker}
              setPicker={setPicker}
              wide
              onChange={(v) => {
                const node = allNodes.find((n) => n.id === v)
                if (!node) {
                  patch({ entityId: '' })
                  return
                }
                patch({
                  entityId: v,
                  namespace: node.namespace || state.namespace,
                  kind: node.kind,
                })
              }}
            />
          </BuilderRow>

          <div className={styles.builderFooter}>
            {selectedNode ? (
              <span className={styles.builderHint}>
                Selected: <strong>{nodeLabel(selectedNode)}</strong>
              </span>
            ) : (
              <span className={styles.builderHint}>No entity selected</span>
            )}
            <button type="button" className={styles.applyButton} onClick={() => setEditing(false)}>
              Apply
            </button>
          </div>
        </div>
      )}
    </div>
  )
}

interface PathSegmentProps {
  id: Exclude<PickerId, null>
  label: string
  value: string
  options: FilterOption[]
  selected: string
  color?: string
  picker: PickerId
  setPicker: (id: PickerId) => void
  onSelect: (value: string) => void
  width?: number
}

function PathSegment({
  id,
  label,
  value,
  color,
  options,
  selected,
  picker,
  setPicker,
  onSelect,
  width = 280,
}: PathSegmentProps) {
  const open = picker === id
  const buttonRef = useRef<HTMLButtonElement>(null)
  return (
    <span className={styles.segmentWrap}>
      <button
        ref={buttonRef}
        type="button"
        className={styles.segment}
        data-open={open}
        onClick={() => setPicker(open ? null : id)}
        style={color ? { color } : undefined}
      >
        <span className={styles.segmentLabel}>{label}</span>
        <span className={styles.segmentValue}>{value}</span>
        <Chevron />
      </button>
      <SearchablePopover
        open={open}
        options={options}
        selected={selected}
        onSelect={onSelect}
        onClose={() => setPicker(null)}
        width={width}
        anchorRef={buttonRef}
      />
    </span>
  )
}

function BuilderRow({ keyword, children }: { keyword: string; children: React.ReactNode }) {
  return (
    <div className={styles.builderRow}>
      <span className={styles.builderKeyword}>{keyword}</span>
      {children}
    </div>
  )
}

interface BuilderComboboxProps {
  id: Exclude<PickerId, null> | 'builder-namespace' | 'builder-kind' | 'builder-status' | 'builder-entity'
  label: string
  value: string
  options: FilterOption[]
  picker: PickerId
  setPicker: (id: PickerId) => void
  onChange: (value: string) => void
  wide?: boolean
}

function BuilderCombobox({
  id,
  label,
  value,
  options,
  picker,
  setPicker,
  onChange,
  wide,
}: BuilderComboboxProps) {
  const open = picker === (id as PickerId)
  const current = options.find((o) => o.value === value)
  const buttonRef = useRef<HTMLButtonElement>(null)
  return (
    <span className={styles.combobox} data-wide={wide ? 'true' : 'false'} data-open={open}>
      <span className={styles.comboboxLabel}>{label}</span>
      <span className={styles.comboboxEq}>=</span>
      <button
        ref={buttonRef}
        type="button"
        className={styles.comboboxButton}
        data-set={Boolean(value)}
        onClick={() => setPicker(open ? null : (id as PickerId))}
      >
        <span className={styles.comboboxValue}>{current?.label ?? 'any'}</span>
        <Chevron />
      </button>
      <SearchablePopover
        open={open}
        options={options}
        selected={value}
        onSelect={onChange}
        onClose={() => setPicker(null)}
        width={wide ? 380 : 300}
        anchorRef={buttonRef}
      />
    </span>
  )
}

function Slash() {
  return <span className={styles.slash}>/</span>
}

function Chevron() {
  return (
    <svg width={10} height={10} viewBox="0 0 10 10" fill="none" stroke="currentColor" strokeWidth={1.5} aria-hidden>
      <path d="M2 4l3 3 3-3" />
    </svg>
  )
}

function FilterIcon() {
  return (
    <svg width={13} height={13} viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth={1.4} aria-hidden>
      <path d="M2 3h12M4 8h8M6 13h4" />
    </svg>
  )
}
