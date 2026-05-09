import { ConnectionStatusButton } from './ConnectionStatusPopover'
import styles from './TopBar.module.css'

export function TopBar() {
  return (
    <header className={styles.topBar}>
      <div className={styles.brand}>
        <span className={styles.logo}>OKSG</span>
        <span className={styles.subtitle}>VISUALIZER</span>
      </div>
      <div className={styles.indicators}>
        <ConnectionStatusButton />
      </div>
    </header>
  )
}
