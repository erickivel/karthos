import { useRef, useState, type RefObject } from 'react'
import { useClickOutside } from '@/hooks/useClickOutside'
import { useSystemStatus } from '@/hooks/useSystemStatus'
import { useGraphSnapshot } from '@/hooks/useGraphSnapshot'
import { useAppStore } from '@/stores/useAppStore'
import { formatAge } from '@/utils/formatters'
import type { ComponentStatus, SystemStatus } from '@/types/status'
import styles from './ConnectionStatusPopover.module.css'

const COMPONENT_LABELS: Record<keyof SystemStatus['components'], string> = {
  metric_store: 'Metric store',
  graph_store: 'Graph store',
  metadata_store: 'Metadata store',
  entity_registry: 'Entity registry',
}

interface Props {
  triggerRef: RefObject<HTMLButtonElement | null>
  open: boolean
  onClose: () => void
}

export function ConnectionStatusPopover({ triggerRef, open, onClose }: Props) {
  const popoverRef = useRef<HTMLDivElement>(null)
  useClickOutside([popoverRef, triggerRef], open, onClose)

  const connectionStatus = useAppStore((s) => s.connectionStatus)
  const { data: status, isError: statusError } = useSystemStatus()
  const { data: snapshot } = useGraphSnapshot()

  if (!open) return null

  const reasons = status?.reasons ?? []
  const components = status?.components

  return (
    <div ref={popoverRef} className={styles.popover} role="dialog" aria-label="Connection status">
      <header className={styles.header}>
        <span className={styles.dot} data-status={connectionStatus} />
        <span className={styles.headerLabel}>{connectionStatus.toUpperCase()}</span>
      </header>

      {statusError && (
        <p className={styles.errorLine}>
          /api/v1/status request failed — backend may be down or the proxy is unreachable.
        </p>
      )}

      {reasons.length > 0 && (
        <section className={styles.section}>
          <h4 className={styles.sectionTitle}>Why</h4>
          <ul className={styles.reasonList}>
            {reasons.map((r) => (
              <li key={r}>{r}</li>
            ))}
          </ul>
        </section>
      )}

      <section className={styles.section}>
        <h4 className={styles.sectionTitle}>Polling</h4>
        <ul className={styles.kvList}>
          <li>
            <span className={styles.k}>System status</span>
            <span className={styles.v}>/api/v1/status · 30s</span>
          </li>
          <li>
            <span className={styles.k}>Graph snapshot</span>
            <span className={styles.v}>
              /api/v1/graph
              {snapshot?.summary.snapshot_age_seconds !== undefined
                ? ` · ${formatAge(snapshot.summary.snapshot_age_seconds)}`
                : ''}
            </span>
          </li>
        </ul>
      </section>

      {components && (
        <section className={styles.section}>
          <h4 className={styles.sectionTitle}>Backend components</h4>
          <ul className={styles.kvList}>
            {(Object.entries(components) as [
              keyof SystemStatus['components'],
              ComponentStatus,
            ][]).map(([key, value]) => (
              <li key={key}>
                <span className={styles.k}>{COMPONENT_LABELS[key]}</span>
                <span className={styles.v} data-component-status={value}>
                  {value}
                </span>
              </li>
            ))}
          </ul>
        </section>
      )}

      {status && (
        <section className={styles.section}>
          <h4 className={styles.sectionTitle}>Snapshot</h4>
          <ul className={styles.kvList}>
            <li>
              <span className={styles.k}>Freshness</span>
              <span className={styles.v} data-stale={status.data_stale ? 'true' : 'false'}>
                {formatAge(status.snapshot_freshness_seconds)}
                {status.data_stale ? ' · stale' : ''}
              </span>
            </li>
            <li>
              <span className={styles.k}>Uptime</span>
              <span className={styles.v}>{formatAge(status.uptime_seconds)}</span>
            </li>
          </ul>
        </section>
      )}
    </div>
  )
}

export function ConnectionStatusButton() {
  const connectionStatus = useAppStore((s) => s.connectionStatus)
  const [open, setOpen] = useState(false)
  const buttonRef = useRef<HTMLButtonElement>(null)

  return (
    <span className={styles.wrap}>
      <button
        ref={buttonRef}
        type="button"
        className={styles.trigger}
        data-open={open}
        onClick={() => setOpen((v) => !v)}
        aria-label="Connection status details"
        aria-expanded={open}
      >
        <span className={styles.triggerDot} data-status={connectionStatus} />
        <span className={styles.triggerLabel}>
          {connectionStatus === 'connected' ? 'POLLING' : connectionStatus.toUpperCase()}
        </span>
      </button>
      <ConnectionStatusPopover
        triggerRef={buttonRef}
        open={open}
        onClose={() => setOpen(false)}
      />
    </span>
  )
}
