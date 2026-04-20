import { Outlet } from 'react-router-dom'
import { TopBar } from './TopBar'
import { Sidebar } from './Sidebar'
import { StatusBar } from './StatusBar'
import styles from './Shell.module.css'

export function Shell() {
  return (
    <div className={styles.shell}>
      <TopBar />
      <Sidebar />
      <main className={styles.content}>
        <Outlet />
      </main>
      <StatusBar />
    </div>
  )
}
