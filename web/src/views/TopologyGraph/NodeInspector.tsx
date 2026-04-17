import { useNavigate } from 'react-router-dom'
import { useNodeDetail } from '@/hooks/useNodeDetail'
import { useNodeMetricSummary } from '@/hooks/useNodeMetricSummary'
import { useAppStore } from '@/stores/useAppStore'
import { DataChip } from '@/components/shared/DataChip'
import type { GraphNode } from '@/types/graph'
import styles from './NodeInspector.module.css'

function getDetailItems(node: GraphNode) {
  const labels = node.labels ?? {}

  if (node.kind === 'workload') {
    const workloadType =
      labels['k8s.deployment.name'] ? 'Deployment'
      : labels['k8s.statefulset.name'] ? 'StatefulSet'
      : labels['k8s.daemonset.name'] ? 'DaemonSet'
      : labels['k8s.replicaset.name'] ? 'ReplicaSet'
      : 'Workload'

    return [
      { label: 'Type', value: workloadType },
      { label: 'Controller', value: node.name },
      ...(node.namespace ? [{ label: 'Namespace', value: node.namespace }] : []),
    ]
  }

  if (node.kind === 'namespace') {
    return [
      { label: 'Namespace', value: node.name },
      ...(node.cluster ? [{ label: 'Cluster', value: node.cluster }] : []),
    ]
  }

  if (node.kind === 'node') {
    return [
      { label: 'Node', value: node.name },
      ...(node.cluster ? [{ label: 'Cluster', value: node.cluster }] : []),
    ]
  }

  return [
    ...(node.cluster ? [{ label: 'Cluster', value: node.cluster }] : []),
    ...(node.namespace ? [{ label: 'Namespace', value: node.namespace }] : []),
  ]
}

export function NodeInspector() {
  const navigate = useNavigate()
  const selectedEntityId = useAppStore((s) => s.selectedEntityId)
  const setSelectedEntityId = useAppStore((s) => s.setSelectedEntityId)
  const { data } = useNodeDetail(selectedEntityId)

  if (!selectedEntityId || !data) return null

  const { node, edges } = data
  const labels = node.labels ?? {}
  const detailItems = getDetailItems(node)
  const metricSummary = useNodeMetricSummary(node)

  return (
    <div className={styles.panel}>
      <div className={styles.header}>
        <div className={styles.titleRow}>
          <span className={styles.dot} data-status={node.status} />
          <h2 className={styles.name}>{node.name}</h2>
        </div>
        <button
          className={styles.closeBtn}
          onClick={() => setSelectedEntityId(null)}
        >
          x
        </button>
      </div>

      <div className={styles.chips}>
        <DataChip label={node.kind} variant="signal" />
        {node.namespace && <DataChip label={node.namespace} />}
      </div>

      {(detailItems.length > 0 || metricSummary.length > 0) && (
        <section className={styles.section}>
          <h3 className={styles.sectionTitle}>Details</h3>

          {detailItems.length > 0 && (
            <div className={styles.propList}>
              {detailItems.map((item) => (
                <div key={item.label} className={styles.prop}>
                  <span className={styles.propKey}>{item.label}</span>
                  <span className={styles.propVal}>{item.value}</span>
                </div>
              ))}
            </div>
          )}

          {metricSummary.length > 0 && (
            <div className={styles.summaryGrid}>
              {metricSummary.map((item) => (
                <div
                  key={item.label}
                  className={styles.summaryCard}
                  data-tone={item.tone ?? 'muted'}
                >
                  <span className={styles.summaryLabel}>{item.label}</span>
                  <span className={styles.summaryValue}>{item.value}</span>
                </div>
              ))}
            </div>
          )}
        </section>
      )}

      <section className={styles.section}>
        <h3 className={styles.sectionTitle}>Properties</h3>
        <div className={styles.propList}>
          {Object.entries(labels).map(([key, value]) => (
            <div key={key} className={styles.prop}>
              <span className={styles.propKey}>{key}</span>
              <span className={styles.propVal}>{value}</span>
            </div>
          ))}
          {Object.keys(labels).length === 0 && (
            <div className={styles.prop}>
              <span className={styles.propVal}>No properties available</span>
            </div>
          )}
        </div>
      </section>

      {edges.inbound.length > 0 && (
        <section className={styles.section}>
          <h3 className={styles.sectionTitle}>Inbound ({edges.inbound.length})</h3>
          <div className={styles.edgeList}>
            {edges.inbound.map((e) => (
              <button
                key={e.id}
                className={styles.edgeItem}
                onClick={() => e.related_node && setSelectedEntityId(e.related_node.id)}
              >
                <span className={styles.edgeRelation}>{e.relation}</span>
                <span className={styles.edgeEntity}>
                  {e.related_node?.name || String(e.source)}
                </span>
              </button>
            ))}
          </div>
        </section>
      )}

      {edges.outbound.length > 0 && (
        <section className={styles.section}>
          <h3 className={styles.sectionTitle}>Outbound ({edges.outbound.length})</h3>
          <div className={styles.edgeList}>
            {edges.outbound.map((e) => (
              <button
                key={e.id}
                className={styles.edgeItem}
                onClick={() => e.related_node && setSelectedEntityId(e.related_node.id)}
              >
                <span className={styles.edgeRelation}>{e.relation}</span>
                <span className={styles.edgeEntity}>
                  {e.related_node?.name || String(e.target)}
                </span>
              </button>
            ))}
          </div>
        </section>
      )}

      <button
        className={styles.metricsLink}
        onClick={() => navigate(`/metrics?entity=${encodeURIComponent(selectedEntityId)}`)}
      >
        View Full Metrics &rarr;
      </button>
    </div>
  )
}
