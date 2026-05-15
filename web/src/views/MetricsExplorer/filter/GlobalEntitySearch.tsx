import { useEffect, useMemo, useRef, useState } from 'react'
import { useClickOutside } from '@/hooks/useClickOutside'
import type { GraphNode } from '@/types/graph'
import { entityKindLabel } from '@/utils/entityDisplay'
import { applyFilters, nodeHealth, type NodeHealth } from './types'
import styles from './GlobalEntitySearch.module.css'

interface Props {
  nodes: GraphNode[] | undefined
  onSelect: (node: GraphNode) => void
}

const MAX_RESULTS = 80

export function GlobalEntitySearch({ nodes, onSelect }: Props) {
  const [open, setOpen] = useState(false)
  const [q, setQ] = useState('')
  const [hi, setHi] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)
  const wrapRef = useRef<HTMLDivElement>(null)
  useClickOutside(wrapRef, open, () => setOpen(false))

  const matches = useMemo(
    () => applyFilters(nodes, { namespace: '', kind: '', entityId: '', query: q, status: '' }).slice(0, MAX_RESULTS),
    [nodes, q],
  )

  useEffect(() => setHi(0), [q])

  useEffect(() => {
    function handler(e: KeyboardEvent) {
      if ((e.metaKey || e.ctrlKey) && e.key.toLowerCase() === 'k') {
        e.preventDefault()
        inputRef.current?.focus()
        setOpen(true)
      }
    }
    window.addEventListener('keydown', handler)
    return () => window.removeEventListener('keydown', handler)
  }, [])

  const pick = (node: GraphNode) => {
    onSelect(node)
    setQ('')
    setOpen(false)
    inputRef.current?.blur()
  }

  return (
    <div className={styles.wrap} ref={wrapRef}>
      <div className={styles.bar} data-open={open}>
        <SearchIcon />
        <input
          ref={inputRef}
          className={styles.input}
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onFocus={() => setOpen(true)}
          onKeyDown={(e) => {
            if (e.key === 'ArrowDown') {
              e.preventDefault()
              setHi((v) => Math.min(matches.length - 1, v + 1))
            } else if (e.key === 'ArrowUp') {
              e.preventDefault()
              setHi((v) => Math.max(0, v - 1))
            } else if (e.key === 'Enter' && matches[hi]) {
              e.preventDefault()
              pick(matches[hi])
            } else if (e.key === 'Escape') {
              e.preventDefault()
              setOpen(false)
              inputRef.current?.blur()
            }
          }}
          placeholder="Search all Kubernetes entities by name, namespace, kind, or status…"
          aria-label="Search entities"
        />
        <kbd className={styles.kbd}>⌘ K</kbd>
      </div>

      {open && (
        <div className={styles.results} role="listbox">
          {matches.length === 0 && (
            <div className={styles.empty}>
              {q ? 'No entities match this query' : 'Start typing to search the snapshot'}
            </div>
          )}
          {matches.map((node, i) => (
            <button
              key={node.id}
              type="button"
              onMouseDown={(ev) => {
                ev.preventDefault()
                pick(node)
              }}
              onMouseEnter={() => setHi(i)}
              className={styles.row}
              data-active={i === hi}
              role="option"
              aria-selected={i === hi}
            >
              <HealthDot health={nodeHealth(node.status)} />
              <KindBadge kind={node.kind} label={entityKindLabel(node)} />
              <span className={styles.name}>{node.name}</span>
              <span className={styles.ns}>{node.namespace || 'cluster'}</span>
              <span className={styles.status}>{node.status}</span>
            </button>
          ))}
        </div>
      )}
    </div>
  )
}

function HealthDot({ health }: { health: NodeHealth }) {
  return <span className={styles.healthDot} data-health={health} aria-label={`status ${health}`} />
}

function KindBadge({ kind, label }: { kind: string; label: string }) {
  return (
    <span className={styles.kindBadge} data-kind={kind}>
      <span className={styles.kindSwatch} style={{ background: `var(--kind-${kind}, var(--color-text-muted))` }} />
      {label}
    </span>
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
