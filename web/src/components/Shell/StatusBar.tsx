import { useSystemStatus } from '@/hooks/useSystemStatus'
import { useGraphSnapshot } from '@/hooks/useGraphSnapshot'
import { useAppStore } from '@/stores/useAppStore'
import styles from './StatusBar.module.css'

function formatAge(seconds: number): string {
  if (seconds < 60) return `${Math.round(seconds)}s AGO`
  if (seconds < 3600) return `${Math.round(seconds / 60)}m AGO`
  return `${Math.round(seconds / 3600)}h AGO`
}

export function StatusBar() {
  const connectionStatus = useAppStore((s) => s.connectionStatus)
  const { data: status } = useSystemStatus()
  const { data: snapshot } = useGraphSnapshot()

  const systemLabel = connectionStatus === 'connected'
    ? 'BACKEND_OK'
    : connectionStatus === 'degraded'
      ? 'BACKEND_DEGRADED'
      : 'BACKEND_OFFLINE'

  const nodeCount = snapshot?.summary.node_count ?? 0
  const edgeCount = snapshot?.summary.edge_count ?? 0
  const snapshotAge = snapshot?.summary.snapshot_age_seconds

  return (
    <footer className={styles.statusBar}>
      <div className={styles.segment}>
        <span className={styles.dot} data-status={connectionStatus} />
        <span className={styles.label}>{systemLabel}</span>
      </div>

      <div className={styles.segment}>
        <span className={styles.tag}>[OKSG]</span>
        <span className={styles.label}>
          {connectionStatus !== 'disconnected' ? 'BACKEND STATUS' : 'BACKEND UNREACHABLE'}
        </span>
        {snapshotAge !== undefined && (
          <>
            <span className={styles.separator}>·</span>
            <span className={styles.label}>LAST SNAPSHOT {formatAge(snapshotAge)}</span>
          </>
        )}
        <span className={styles.separator}>·</span>
        <span className={styles.label}>{nodeCount} NODES</span>
        <span className={styles.separator}>·</span>
        <span className={styles.label}>{edgeCount} EDGES</span>
      </div>

      <div className={styles.right}>
        <span className={styles.tag}>[NET]</span>
        <span className={styles.label}>
          API {status ? 'oksg-backend:8080' : '--'}
        </span>
      </div>
    </footer>
  )
}
