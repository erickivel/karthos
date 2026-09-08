import { ConnectionStatusButton } from './ConnectionStatusPopover'
import styles from './TopBar.module.css'

export function TopBar() {
  return (
    <header className={styles.topBar}>
      <div className={styles.brand}>
        <img className={styles.mark} src="/karthos-mark.svg" alt="" />
        <div className={styles.wordmark}>
          <span className={styles.logo}>Karthos</span>
          <span className={styles.subtitle}>DATA FOUNDATION</span>
        </div>
      </div>
      <div className={styles.indicators}>
        <ConnectionStatusButton />
      </div>
    </header>
  )
}
