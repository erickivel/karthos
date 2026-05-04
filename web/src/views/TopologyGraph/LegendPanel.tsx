import { X } from 'lucide-react'
import type { EntityKind } from '@/types/graph'
import {
  GRANULARITY_ENTRIES,
  MODE_ENTRIES,
  TRAFFIC_GRANULARITY_ENTRIES,
} from './modes'
import styles from './LegendPanel.module.css'

interface LegendPanelProps {
  onClose: () => void
}

interface KindEntry {
  kind: EntityKind
  label: string
  description: string
}

const KIND_ENTRIES: KindEntry[] = [
  { kind: 'namespace', label: 'Namespace', description: 'Logical grouping of K8s objects' },
  { kind: 'node', label: 'Node', description: 'Physical/virtual host running pods' },
  { kind: 'service', label: 'Service', description: 'Stable network endpoint for workloads' },
  { kind: 'workload', label: 'Workload', description: 'Deployment / StatefulSet / DaemonSet' },
  { kind: 'pod', label: 'Pod', description: 'One or more containers sharing a network' },
  { kind: 'container', label: 'Container', description: 'Process running inside a pod' },
  { kind: 'ingress', label: 'Ingress', description: 'External HTTP(S) routing rules' },
  { kind: 'configmap', label: 'ConfigMap', description: 'Non-sensitive configuration data' },
  { kind: 'secret', label: 'Secret', description: 'Sensitive configuration data' },
  { kind: 'pvc', label: 'PVC', description: 'Persistent volume claim (storage)' },
]

export function LegendPanel({ onClose }: LegendPanelProps) {
  return (
    <div
      className={styles.panel}
      role="dialog"
      aria-label="Topology legend"
      data-testid="topology-legend"
    >
      <header className={styles.header}>
        <span className={styles.title}>Topology legend</span>
        <button
          type="button"
          className={styles.close}
          onClick={onClose}
          aria-label="Close legend"
        >
          <X size={14} strokeWidth={2} />
        </button>
      </header>

      <section className={styles.section}>
        <h3 className={styles.sectionTitle}>Modes</h3>
        <ul className={styles.list}>
          {MODE_ENTRIES.map((m) => (
            <li key={m.value} className={styles.item}>
              <span className={styles.itemLabel}>{m.label}</span>
              <span className={styles.itemDesc}>{m.description}</span>
            </li>
          ))}
        </ul>
      </section>

      <section className={styles.section}>
        <h3 className={styles.sectionTitle}>Granularity (Ownership / Infrastructure)</h3>
        <ul className={styles.list}>
          {GRANULARITY_ENTRIES.map((g) => (
            <li key={g.value} className={styles.item}>
              <span className={styles.itemLabel}>{g.label}</span>
              <span className={styles.itemDesc}>{g.description}</span>
            </li>
          ))}
        </ul>
      </section>

      <section className={styles.section}>
        <h3 className={styles.sectionTitle}>Traffic flows</h3>
        <ul className={styles.list}>
          {TRAFFIC_GRANULARITY_ENTRIES.map((g) => (
            <li key={g.value} className={styles.item}>
              <span className={styles.itemLabel}>{g.label}</span>
              <span className={styles.itemDesc}>{g.description}</span>
            </li>
          ))}
        </ul>
      </section>

      <section className={styles.section}>
        <h3 className={styles.sectionTitle}>Entity colors</h3>
        <ul className={styles.kindList}>
          {KIND_ENTRIES.map((k) => (
            <li key={k.kind} className={styles.kindItem}>
              <span
                className={styles.swatch}
                style={{ background: `var(--kind-${k.kind})` }}
                aria-hidden
              />
              <span className={styles.kindLabel}>{k.label}</span>
              <span className={styles.kindDesc}>{k.description}</span>
            </li>
          ))}
        </ul>
      </section>
    </div>
  )
}
