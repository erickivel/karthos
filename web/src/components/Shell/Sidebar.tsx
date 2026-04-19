import { NavLink, useLocation } from 'react-router-dom'
import { Box, Network, BarChart3 } from 'lucide-react'
import styles from './Sidebar.module.css'

const NAV_ITEMS = [
  { path: '/', label: 'Overview', Icon: Box },
  { path: '/topology', label: 'Topology', Icon: Network },
  { path: '/metrics', label: 'Metrics', Icon: BarChart3 },
] as const

export function Sidebar() {
  const location = useLocation()

  const isActive = (path: string) => {
    if (path === '/') return location.pathname === '/'
    return location.pathname.startsWith(path)
  }

  return (
    <nav className={styles.sidebar}>
      {NAV_ITEMS.map((item) => (
        <NavLink
          key={item.path}
          className={styles.navButton}
          data-active={isActive(item.path)}
          title={item.label}
          aria-label={item.label}
          to={item.path}
        >
          <item.Icon size={18} strokeWidth={1.5} className={styles.icon} />
        </NavLink>
      ))}
    </nav>
  )
}
