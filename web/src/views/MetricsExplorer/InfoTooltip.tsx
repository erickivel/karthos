import { useState } from 'react'
import type { SourceRef } from '@/types/metrics'
import styles from './InfoTooltip.module.css'

interface Props {
  canonicalName: string
  sampleCount: number
  sources?: SourceRef[]
  activeSource?: string
}

export function InfoTooltip({ canonicalName, sampleCount, sources, activeSource }: Props) {
  const [open, setOpen] = useState(false)
  const tooltipId = `metric-tip-${canonicalName.replace(/[^a-z0-9]+/gi, '-')}`
  const sortedSources = sources ? [...sources].sort((a, b) => a.priority - b.priority) : []

  return (
    <span
      className={styles.wrap}
      onMouseEnter={() => setOpen(true)}
      onMouseLeave={() => setOpen(false)}
      onBlur={() => setOpen(false)}
    >
      <button
        type="button"
        className={styles.button}
        aria-label={`Metric details: ${sampleCount} samples, canonical metric ${canonicalName}`}
        aria-describedby={open ? tooltipId : undefined}
        onClick={() => setOpen((v) => !v)}
        onFocus={() => setOpen(true)}
      >
        <svg width={18} height={18} viewBox="0 0 18 18" fill="none" stroke="currentColor" strokeWidth={1.7} strokeLinecap="round" strokeLinejoin="round" aria-hidden>
          <path d="M9 8.2v4.4" />
          <path d="M9 5.4h.01" />
          <path d="M9 16A7 7 0 1 0 9 2a7 7 0 0 0 0 14Z" />
        </svg>
      </button>
      {open && (
        <span id={tooltipId} role="tooltip" className={styles.tooltip}>
          <span className={styles.title}>Metric details</span>
          <span>
            <strong className={styles.value}>{sampleCount}</strong> samples in the selected time range
          </span>
          <span className={styles.muted}>
            Canonical name: <strong className={styles.code}>{canonicalName}</strong>
          </span>
          {sortedSources.length > 0 && (
            <span className={styles.muted}>
              Sources:{' '}
              {sortedSources.map((s, i) => (
                <span key={s.id}>
                  <strong className={s.id === activeSource ? styles.activeSource : styles.code}>{s.id}</strong>
                  {i < sortedSources.length - 1 ? ', ' : ''}
                </span>
              ))}
            </span>
          )}
        </span>
      )}
    </span>
  )
}
