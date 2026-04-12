import { useNavigate } from 'react-router-dom'
import styles from './EntityBreakdown.module.css'

interface EntityBreakdownProps {
  kindCounts: Record<string, number>
}

export function EntityBreakdown({ kindCounts }: EntityBreakdownProps) {
  const navigate = useNavigate()
  const entries = Object.entries(kindCounts).sort(([, a], [, b]) => b - a)
  const maxCount = entries.length > 0 ? entries[0]![1] : 1

  return (
    <div className={styles.container}>
      <h2 className={styles.heading}>Entity Breakdown</h2>
      <div className={styles.list}>
        {entries.map(([kind, count]) => (
          <button
            key={kind}
            className={styles.row}
            onClick={() => navigate(`/topology?kind=${kind}`)}
          >
            <span className={styles.kind}>{kind}</span>
            <div className={styles.barTrack}>
              <div
                className={styles.barFill}
                style={{ width: `${(count / maxCount) * 100}%` }}
              />
            </div>
            <span className={styles.count}>{count}</span>
          </button>
        ))}
      </div>
    </div>
  )
}
