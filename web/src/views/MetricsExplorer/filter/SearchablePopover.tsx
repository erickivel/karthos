import { useEffect, useMemo, useRef, useState } from 'react'
import type { FilterOption, NodeHealth } from './types'
import styles from './SearchablePopover.module.css'

interface SearchablePopoverProps {
  open: boolean
  options: FilterOption[]
  selected: string
  onSelect: (value: string) => void
  onClose: () => void
  width?: number
}

export function SearchablePopover({
  open,
  options,
  selected,
  onSelect,
  onClose,
  width = 280,
}: SearchablePopoverProps) {
  const [q, setQ] = useState('')
  const [hi, setHi] = useState(0)
  const inputRef = useRef<HTMLInputElement>(null)

  const filtered = useMemo(() => {
    const query = q.trim().toLowerCase()
    if (!query) return options
    return options.filter((o) =>
      `${o.label} ${o.meta ?? ''} ${o.value}`.toLowerCase().includes(query),
    )
  }, [options, q])

  useEffect(() => {
    if (!open) return
    setQ('')
    setHi(0)
    const id = window.setTimeout(() => inputRef.current?.focus(), 0)
    return () => window.clearTimeout(id)
  }, [open])

  if (!open) return null

  const pick = (value: string) => {
    onSelect(value)
    onClose()
  }

  return (
    <div className={styles.popover} style={{ width }} role="listbox">
      <div className={styles.searchRow}>
        <SearchIcon />
        <input
          ref={inputRef}
          value={q}
          onChange={(e) => setQ(e.target.value)}
          onKeyDown={(e) => {
            if (e.key === 'ArrowDown') {
              e.preventDefault()
              setHi((v) => Math.min(filtered.length - 1, v + 1))
            } else if (e.key === 'ArrowUp') {
              e.preventDefault()
              setHi((v) => Math.max(0, v - 1))
            } else if (e.key === 'Enter' && filtered[hi]) {
              e.preventDefault()
              pick(filtered[hi].value)
            } else if (e.key === 'Escape') {
              e.preventDefault()
              onClose()
            }
          }}
          placeholder="Filter options…"
          className={styles.input}
          aria-label="Filter options"
        />
      </div>
      <div className={styles.list}>
        {filtered.length === 0 && <div className={styles.empty}>No options</div>}
        {filtered.map((o, i) => (
          <button
            key={`${o.value}|${o.label}`}
            type="button"
            onMouseDown={(ev) => {
              ev.preventDefault()
              pick(o.value)
            }}
            onMouseEnter={() => setHi(i)}
            className={styles.option}
            data-active={i === hi || selected === o.value}
            role="option"
            aria-selected={selected === o.value}
          >
            <OptionDecoration health={o.health} color={o.color} />
            <span className={styles.optionLabel}>{o.label}</span>
            {o.meta && <span className={styles.optionMeta}>{o.meta}</span>}
          </button>
        ))}
      </div>
    </div>
  )
}

function OptionDecoration({ health, color }: { health?: NodeHealth; color?: string }) {
  if (health) {
    return <span className={styles.healthDot} data-health={health} />
  }
  if (color) {
    return <span className={styles.colorChip} style={{ background: color }} />
  }
  return <span className={styles.spacer} />
}

function SearchIcon() {
  return (
    <svg width={14} height={14} viewBox="0 0 16 16" fill="none" stroke="currentColor" strokeWidth={1.5} aria-hidden>
      <circle cx="7" cy="7" r="4.5" />
      <path d="M10.5 10.5 14 14" />
    </svg>
  )
}
