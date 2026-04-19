import type { MetricCategory } from '@/types/metrics'
import styles from './CategoryTabs.module.css'

const CATEGORIES: { label: string; value: MetricCategory }[] = [
  { label: 'Resource', value: 'resource' },
  { label: 'Traffic', value: 'traffic' },
  { label: 'Latency', value: 'latency' },
  { label: 'Error', value: 'error' },
  { label: 'Capacity', value: 'capacity' },
  { label: 'Health', value: 'health' },
]

interface CategoryTabsProps {
  active: MetricCategory
  categories?: MetricCategory[]
  onChange: (cat: MetricCategory) => void
}

export function CategoryTabs({ active, categories, onChange }: CategoryTabsProps) {
  const visibleCategories = categories ?? CATEGORIES.map((cat) => cat.value)

  return (
    <div className={styles.tabs}>
      {CATEGORIES.filter((cat) => visibleCategories.includes(cat.value)).map((cat) => (
        <button
          key={cat.value}
          type="button"
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
