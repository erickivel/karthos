import { DataChip } from '@/components/shared/DataChip'
import type { CapabilityProfile } from '@/api/schemas'
import styles from './DiscoverySources.module.css'

interface DiscoverySourcesProps {
  discovery: CapabilityProfile | undefined
  snapshotCreatedAt: string | undefined
  snapshotVersion: { major: number; minor: number } | undefined
}

function formatTimeAgo(iso: string): string {
  const diff = Date.now() - new Date(iso).getTime()
  const minutes = Math.floor(diff / 60_000)
  if (minutes < 60) return `${minutes}m ago`
  const hours = Math.floor(minutes / 60)
  if (hours < 24) return `${hours}h ago`
  return `${Math.floor(hours / 24)}d ago`
}

export function DiscoverySources({ discovery, snapshotCreatedAt, snapshotVersion }: DiscoverySourcesProps) {
  return (
    <div className={styles.container}>
      <div className={styles.section}>
        <h3 className={styles.heading}>Discovery Sources</h3>
        <div className={styles.chips}>
          {discovery?.capabilities.map((capability) => (
            <DataChip
              key={capability.source_id}
              label={`${capability.source_id} · ${capability.kind} · ${capability.status}`}
              variant={capability.status === 'available' ? 'signal' : 'info'}
            />
          ))}
          {(!discovery?.capabilities || discovery.capabilities.length === 0) && (
            <span className={styles.empty}>No sources</span>
          )}
        </div>
      </div>
      {discovery?.capabilities && (
        <div className={styles.section}>
          <h3 className={styles.heading}>Detected Signals</h3>
          <div className={styles.chips}>
            {discovery.capabilities.flatMap((capability) => capability.signals.map((signal) => (
              <DataChip key={`${capability.source_id}:${signal}`} label={`${capability.source_id}: ${signal}`} />
            )))}
          </div>
        </div>
      )}

      <div className={styles.section}>
        <h3 className={styles.heading}>Graph Snapshot</h3>
        <div className={styles.meta}>
          {snapshotCreatedAt && (
            <span className={styles.metaItem}>
              Created {formatTimeAgo(snapshotCreatedAt)}
            </span>
          )}
          {snapshotVersion && (
            <span className={styles.metaItem}>
              v{snapshotVersion.major}.{snapshotVersion.minor}
            </span>
          )}
        </div>
      </div>
    </div>
  )
}
