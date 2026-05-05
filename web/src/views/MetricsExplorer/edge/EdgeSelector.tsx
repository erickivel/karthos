import { useEffect, useMemo, useRef, useState } from 'react'
import type { GraphEdge, GraphNode } from '@/types/graph'
import { nodeHealth, type NodeHealth } from '../filter/types'
import styles from './EdgeSelector.module.css'

interface Props {
  nodes: GraphNode[] | undefined
  edges: GraphEdge[] | undefined
  selectedSource: string
  selectedTarget: string
  onSelect: (sourceId: string, targetId: string) => void
}

interface EdgeRow {
  id: string
  sourceId: string
  targetId: string
  source: GraphNode | null
  target: GraphNode | null
  relation: string
}

const MAX_RESULTS = 120

export function EdgeSelector({ nodes, edges, selectedSource, selectedTarget, onSelect }: Props) {
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState('')
  const [hi, setHi] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)

  const nodeById = useMemo(() => new Map((nodes ?? []).map((n) => [n.id, n])), [nodes])

  const callsEdges = useMemo<EdgeRow[]>(() => {
    const seen = new Set<string>()
    return (edges ?? [])
      .filter((e) => e.relation === 'calls')
      .map<EdgeRow>((e) => ({
        id: `${e.source}|${e.target}`,
        sourceId: e.source,
        targetId: e.target,
        source: nodeById.get(e.source) ?? null,
        target: nodeById.get(e.target) ?? null,
        relation: e.relation,
      }))
      .filter((r) => {
        if (seen.has(r.id)) return false
        seen.add(r.id)
        return true
      })
      .sort((a, b) => {
        const an = a.source?.name ?? a.sourceId
        const bn = b.source?.name ?? b.sourceId
        return an.localeCompare(bn)
      })
  }, [edges, nodeById])

  const filtered = useMemo<EdgeRow[]>(() => {
    const query = q.trim().toLowerCase()
    if (!query) return callsEdges.slice(0, MAX_RESULTS)
    return callsEdges
      .filter((r) => {
        const hay = [
          r.source?.name,
          r.source?.namespace,
          r.target?.name,
          r.target?.namespace,
          r.sourceId,
          r.targetId,
        ]
          .filter(Boolean)
          .join(' ')
          .toLowerCase()
        return hay.includes(query)
      })
      .slice(0, MAX_RESULTS)
  }, [callsEdges, q])

  useEffect(() => setHi(0), [q])

  const currentLabel = useMemo(() => {
    if (!selectedSource || !selectedTarget) return ''
    const s = nodeById.get(selectedSource)
    const t = nodeById.get(selectedTarget)
    return `${s?.name ?? selectedSource} → ${t?.name ?? selectedTarget}`
  }, [nodeById, selectedSource, selectedTarget])

  const pick = (row: EdgeRow) => {
    onSelect(row.sourceId, row.targetId)
    setQ('')
    setOpen(false)
    inputRef.current?.blur()
  }

  return (
    <div className={styles.wrap}>
      <div className={styles.bar} data-open={open}>
        <ArrowIcon />
        <input
          ref={inputRef}
          className={styles.input}
          // While `q` is empty, show the current selection so the field is
          // never blank with a picked edge. When the user types, `q` takes
          // over. Filtering always reads `q`, so an empty `q` means
          // "show all edges" — independent of what's displayed.
          value={q || currentLabel}
          onChange={(e) => setQ(e.target.value)}
          onFocus={(e) => {
            // Pre-select the displayed label so the user's first keystroke
            // replaces it (no manual ⌘A needed). No-op when the input is
            // empty.
            e.currentTarget.select()
            setOpen(true)
          }}
          onBlur={() => window.setTimeout(() => setOpen(false), 120)}
          onKeyDown={(e) => {
            if (e.key === 'ArrowDown') {
              e.preventDefault()
              setHi((v) => Math.min(filtered.length - 1, v + 1))
            } else if (e.key === 'ArrowUp') {
              e.preventDefault()
              setHi((v) => Math.max(0, v - 1))
            } else if (e.key === 'Enter' && filtered[hi]) {
              e.preventDefault()
              pick(filtered[hi])
            } else if (e.key === 'Escape') {
              e.preventDefault()
              setOpen(false)
              inputRef.current?.blur()
            }
          }}
          placeholder={
            callsEdges.length === 0
              ? 'No calls edges in current snapshot'
              : 'Search edges by source, target, or namespace…'
          }
          aria-label="Select edge"
        />
        <span className={styles.count}>{callsEdges.length} edges</span>
      </div>

      {open && (
        <div className={styles.results} role="listbox">
          {filtered.length === 0 && (
            <div className={styles.empty}>
              {q ? 'No edges match this query' : 'No calls edges available'}
            </div>
          )}
          {filtered.map((r, i) => (
            <button
              key={r.id}
              type="button"
              onMouseDown={(ev) => {
                ev.preventDefault()
                pick(r)
              }}
              onMouseEnter={() => setHi(i)}
              className={styles.row}
              data-active={i === hi || (r.sourceId === selectedSource && r.targetId === selectedTarget)}
              role="option"
              aria-selected={r.sourceId === selectedSource && r.targetId === selectedTarget}
            >
              <Endpoint
                node={r.source}
                fallback={r.sourceId}
                health={r.source ? nodeHealth(r.source.status) : 'degraded'}
              />
              <span className={styles.arrow}>→</span>
              <Endpoint
                node={r.target}
                fallback={r.targetId}
                health={r.target ? nodeHealth(r.target.status) : 'degraded'}
              />
              <span className={styles.relation}>{r.relation}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

function Endpoint({ node, fallback, health }: { node: GraphNode | null; fallback: string; health: NodeHealth }) {
  return (
    <span className={styles.endpoint}>
      <span className={styles.healthDot} data-health={health} />
      <span
        className={styles.kindSwatch}
        style={{ background: node ? `var(--kind-${node.kind}, var(--color-text-muted))` : 'var(--color-text-muted)' }}
      />
      <span className={styles.endpointName}>{node ? node.name : fallback}</span>
      {node?.namespace && <span className={styles.endpointNs}>{node.namespace}</span>}
    </span>
  )
}

function ArrowIcon() {
  return (
    <svg width={16} height={16} viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth={1.5} aria-hidden>
      <path d="M2 8h12M10 4l4 4-4 4" />
    </svg>
  )
}
