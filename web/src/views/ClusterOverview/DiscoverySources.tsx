import { DataChip } from '@/components/shared/DataChip'
import type { DiscoverySummary } from '@/types/status'
import styles from './DiscoverySources.module.css'

interface DiscoverySourcesProps {
  discovery: DiscoverySummary | undefined
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
          {discovery?.sources.map((src) => (
            <DataChip key={src.name} label={src.name} variant="info" />
          ))}
          {(!discovery?.sources || discovery.sources.length === 0) && (
            <span className={styles.empty}>No sources</span>
          )}
        </div>
      </div>

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
