import type { MetricCategory } from '@/types/metrics'
import styles from './CategoryTabs.module.css'

const CATEGORIES: { label: string; value: MetricCategory }[] = [
  { label: 'Resource', value: 'resource' },
  { label: 'Traffic', value: 'traffic' },
  { label: 'Latency', value: 'latency' },
  { label: 'Error', value: 'error' },
  { label: 'Capacity', value: 'capacity' },
]

interface CategoryTabsProps {
  active: MetricCategory
  onChange: (cat: MetricCategory) => void
}

export function CategoryTabs({ active, onChange }: CategoryTabsProps) {
  return (
    <div className={styles.tabs}>
      {CATEGORIES.map((cat) => (
        <button
          key={cat.value}
          className={styles.tab}
          data-active={cat.value === active}
          onClick={() => onChange(cat.value)}
        >
          {cat.label}
        </button>
      ))}
    </div>
  )
}
