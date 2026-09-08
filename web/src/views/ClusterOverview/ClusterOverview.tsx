import { useDiscovery } from '@/hooks/useDiscovery'
import { useGraphSnapshot } from '@/hooks/useGraphSnapshot'
import { useSystemStatus } from '@/hooks/useSystemStatus'
import { KpiCard } from './KpiCard'
import { EntityBreakdown } from './EntityBreakdown'
import { RelationTypes } from './RelationTypes'
import { DiscoverySources } from './DiscoverySources'
import { HealthSummary } from './HealthSummary'
import { overviewKpis } from './overviewModel'
import { ApiContractError } from '@/api/client'
import styles from './ClusterOverview.module.css'

export function ClusterOverview() {
  const { data: discovery, error: discoveryError } = useDiscovery()
  const { data: snapshot, error: snapshotError } = useGraphSnapshot()
  const { data: status, error: statusError } = useSystemStatus()
  const errors = [discoveryError, snapshotError, statusError].filter((error): error is Error => error instanceof Error)

  return (
    <div className={styles.container}>
      <header className={styles.header}>
        <h1 className={styles.title}>
          {discovery?.cluster_name ?? 'Cluster'}
        </h1>
        {discovery?.discovered_at && (
          <span className={styles.timestamp}>
            Last discovery: {new Date(discovery.discovered_at).toLocaleString()}
          </span>
        )}
      </header>

      {errors.map((error) => (
        <p key={error.message} className={styles.apiError} role="alert">
          {error instanceof ApiContractError
            ? `API contract mismatch: ${error.message}`
            : `Unable to load current cluster data: ${error.message}`}
        </p>
      ))}

      <div className={styles.kpiRow}>
        {overviewKpis(snapshot?.summary).map(([label, value]) => (
          <KpiCard key={label} label={label} value={value} />
        ))}
      </div>

      <HealthSummary healthSummary={status?.health_summary} />

      <div className={styles.body}>
        <EntityBreakdown kindCounts={snapshot?.summary.kind_counts ?? {}} />
        <RelationTypes relationCounts={snapshot?.summary.relation_counts ?? {}} />
      </div>

      <DiscoverySources
        discovery={discovery}
        snapshotCreatedAt={snapshot?.created_at}
        snapshotVersion={snapshot?.version}
      />
    </div>
  )
}
