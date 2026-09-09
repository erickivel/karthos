import type { HealthSummary as HealthSummaryData } from '@/types/status'
import { KpiCard } from './KpiCard'
import { podHealthBuckets } from './overviewModel'
import styles from './HealthSummary.module.css'

interface HealthSummaryProps {
  healthSummary?: HealthSummaryData
}

export function HealthSummary({ healthSummary }: HealthSummaryProps) {
  if (!healthSummary) return null

  return (
    <section className={styles.section}>
      <div className={styles.header}>
        <h2 className={styles.title}>Runtime Health</h2>
        <span className={styles.subtitle}>
          Current quality and pod phase summary from `/api/v1/status`
        </span>
      </div>

      <div className={styles.grid}>
        <KpiCard
          label="Healthy Nodes"
          value={healthSummary.nodes.healthy}
          accent="signal"
        />
        <KpiCard
          label="Unhealthy Nodes"
          value={healthSummary.nodes.unhealthy}
          accent={healthSummary.nodes.unhealthy > 0 ? 'alert' : 'muted'}
        />
        {podHealthBuckets(healthSummary.pods).map(([label, value, accent]) => (
          <KpiCard key={label} label={label} value={value} accent={accent} />
        ))}
      </div>
      <div className={styles.quality}>
        Health quality: nodes <strong>{healthSummary.nodes.status}</strong>, pods <strong>{healthSummary.pods.status}</strong>
      </div>
    </section>
  )
}
