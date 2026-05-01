import type { HealthSummary as HealthSummaryData } from '@/types/status'
import { KpiCard } from './KpiCard'
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
          Latest node readiness and topology pod state from `/status`
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
        <KpiCard
          label="Running Pods"
          value={healthSummary.pods.running}
          accent="signal"
        />
        <KpiCard
          label="Pending Pods"
          value={healthSummary.pods.pending}
          accent={healthSummary.pods.pending > 0 ? 'info' : 'muted'}
        />
        <KpiCard
          label="Failed Pods"
          value={healthSummary.pods.failed}
          accent={healthSummary.pods.failed > 0 ? 'alert' : 'muted'}
        />
        <KpiCard
          label="Unknown Pods"
          value={healthSummary.pods.unknown}
          accent={healthSummary.pods.unknown > 0 ? 'info' : 'muted'}
        />
      </div>
    </section>
  )
}
