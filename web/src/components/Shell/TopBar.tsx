import { useAppStore } from '@/stores/useAppStore'
import styles from './TopBar.module.css'

export function TopBar() {
  const connectionStatus = useAppStore((s) => s.connectionStatus)

  return (
    <header className={styles.topBar}>
      <div className={styles.brand}>
        <span className={styles.logo}>OKSG</span>
        <span className={styles.subtitle}>VISUALIZER</span>
      </div>
      <div className={styles.indicators}>
        <span className={styles.pollingDot} data-status={connectionStatus} />
        <span className={styles.statusLabel}>
          {connectionStatus === 'connected' ? 'POLLING' : connectionStatus.toUpperCase()}
        </span>
      </div>
    </header>
  )
}
