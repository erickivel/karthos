import { useDiscovery } from '@/hooks/useDiscovery'
import { useGraphSnapshot } from '@/hooks/useGraphSnapshot'
import { useSystemStatus } from '@/hooks/useSystemStatus'
import { KpiCard } from './KpiCard'
import { EntityBreakdown } from './EntityBreakdown'
import { RelationTypes } from './RelationTypes'
import { DiscoverySources } from './DiscoverySources'
import { HealthSummary } from './HealthSummary'
import styles from './ClusterOverview.module.css'

export function ClusterOverview() {
  const { data: discovery } = useDiscovery()
  const { data: snapshot } = useGraphSnapshot()
  const { data: status } = useSystemStatus()

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

      <div className={styles.kpiRow}>
        <KpiCard label="Nodes" value={discovery?.node_count ?? 0} />
        <KpiCard label="Namespaces" value={discovery?.namespace_count ?? 0} />
        <KpiCard label="Workloads" value={discovery?.workload_count ?? 0} />
        <KpiCard label="Services" value={discovery?.service_count ?? 0} />
        <KpiCard label="Graph Edges" value={snapshot?.summary.edge_count ?? 0} />
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
