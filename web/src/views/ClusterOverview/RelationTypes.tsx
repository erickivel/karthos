import { useNavigate } from 'react-router-dom'
import styles from './RelationTypes.module.css'

const RELATION_DESCRIPTIONS: Record<string, string> = {
  calls: 'Service-to-service communication (from OTLP traces)',
  runs_on: 'Pod scheduled on a node (from kubeletstats)',
  contains: 'Parent-child containment (namespace→pod, pod→container)',
  exposes: 'Service exposes a workload (via K8s selector matching)',
  routes_to: 'Ingress routes to a service (from ingress rules)',
  uses: 'Workload references a ConfigMap or Secret',
  mounts: 'Workload mounts a PVC',
}

interface RelationTypesProps {
  relationCounts: Record<string, number>
}

export function RelationTypes({ relationCounts }: RelationTypesProps) {
  const navigate = useNavigate()
  const entries = Object.entries(relationCounts).sort(([, a], [, b]) => b - a)

  return (
    <div className={styles.container}>
      <h2 className={styles.heading}>Relation Types</h2>
      <div className={styles.list}>
        {entries.map(([relation, count]) => (
          <button
            key={relation}
            className={styles.row}
            onClick={() => navigate(`/topology?relation=${relation}`)}
          >
            <div className={styles.header}>
              <span className={styles.name}>{relation}</span>
              <span className={styles.count}>{count}</span>
            </div>
            <span className={styles.description}>
              {RELATION_DESCRIPTIONS[relation] ?? ''}
            </span>
          </button>
        ))}
      </div>
    </div>
  )
}
