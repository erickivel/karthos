import { X } from 'lucide-react'
import type { EntityKind } from '@/types/graph'
import {
  INFRASTRUCTURE_GRANULARITY_ENTRIES,
  MODE_ENTRIES,
  OWNERSHIP_GRANULARITY_ENTRIES,
  TRAFFIC_GRANULARITY_ENTRIES,
  type ModeEntry,
} from './modes'
import type { Granularity, ProjectionMode } from './projection'
import styles from './LegendPanel.module.css'

interface LegendPanelProps {
  onClose: () => void
}

interface KindEntry {
  kind: EntityKind
  label: string
  description: string
}

interface ModeSection {
  mode: ProjectionMode
  title: string
  description: string
  granularities: ModeEntry<Granularity>[]
}

// Per-mode entity allow-lists, kept in sync with `LAYER_SPECS` /
// `INFRASTRUCTURE_VISIBLE_KINDS` / `TRAFFIC_FLOW_SPECS` in projection.ts. Used
// to scope the entity-color section so the legend only shows kinds that
// actually appear in the active mode.
const KINDS_BY_MODE: Record<ProjectionMode, EntityKind[]> = {
  ownership: [
    'namespace',
    'workload',
    'service',
    'ingress',
    'pod',
    'container',
    'configmap',
    'secret',
    'pvc',
  ],
  infrastructure: ['node', 'pod', 'container'],
  traffic: ['ingress', 'service', 'workload', 'pod'],
}

const KIND_ENTRIES: Record<EntityKind, KindEntry> = {
  cluster: {
    kind: 'cluster',
    label: 'Cluster',
    description: 'Top-level Kubernetes cluster',
  },
  namespace: {
    kind: 'namespace',
    label: 'Namespace',
    description: 'Logical grouping of K8s objects',
  },
  node: {
    kind: 'node',
    label: 'Node',
    description: 'Physical/virtual host running pods',
  },
  service: {
    kind: 'service',
    label: 'Service',
    description: 'Stable network endpoint for workloads',
  },
  workload: {
    kind: 'workload',
    label: 'Workload',
    description: 'Deployment / StatefulSet / DaemonSet',
  },
  pod: {
    kind: 'pod',
    label: 'Pod',
    description: 'One or more containers sharing a network',
  },
  container: {
    kind: 'container',
    label: 'Container',
    description: 'Process running inside a pod',
  },
  ingress: {
    kind: 'ingress',
    label: 'Ingress',
    description: 'External HTTP(S) routing rules',
  },
  endpoint: {
    kind: 'endpoint',
    label: 'Endpoint',
    description: 'Backing IPs for a service',
  },
  configmap: {
    kind: 'configmap',
    label: 'ConfigMap',
    description: 'Non-sensitive configuration data',
  },
  secret: {
    kind: 'secret',
    label: 'Secret',
    description: 'Sensitive configuration data',
  },
  pvc: {
    kind: 'pvc',
    label: 'PVC',
    description: 'Persistent volume claim (storage)',
  },
  hpa: {
    kind: 'hpa',
    label: 'HPA',
    description: 'Horizontal pod autoscaler',
  },
}

const MODE_SECTIONS: ModeSection[] = [
  {
    mode: 'ownership',
    title: 'Ownership',
    description:
      'K8s object hierarchy per namespace — namespaces stack vertically; each tree shows what owns what.',
    granularities: OWNERSHIP_GRANULARITY_ENTRIES,
  },
  {
    mode: 'infrastructure',
    title: 'Infrastructure',
    description:
      'Where compute runs: nodes, pods, and containers as nested rectangles. PVCs / Secrets / ConfigMaps live outside this view.',
    granularities: INFRASTRUCTURE_GRANULARITY_ENTRIES,
  },
  {
    mode: 'traffic',
    title: 'Traffic',
    description:
      'Service-centric view that follows observed calls. The granularity picker swaps the flow shown — see below.',
    granularities: TRAFFIC_GRANULARITY_ENTRIES,
  },
]

export function LegendPanel({ onClose }: LegendPanelProps) {
  const allKinds = Array.from(
    new Set<EntityKind>(MODE_SECTIONS.flatMap((s) => KINDS_BY_MODE[s.mode])),
  )

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

      {MODE_SECTIONS.map((section) => (
        <section key={section.mode} className={styles.section}>
          <h3 className={styles.sectionTitle}>{section.title} layers</h3>
          <p className={styles.sectionLead}>{section.description}</p>
          <ul className={styles.list}>
            {section.granularities.map((g) => (
              <li key={g.value} className={styles.item}>
                <span className={styles.itemLabel}>{g.label}</span>
                <span className={styles.itemDesc}>{g.description}</span>
              </li>
            ))}
          </ul>
          <p className={styles.sectionFoot}>
            Visible entities: {KINDS_BY_MODE[section.mode].map((k) => KIND_ENTRIES[k].label).join(' · ')}
          </p>
        </section>
      ))}

      <section className={styles.section}>
        <h3 className={styles.sectionTitle}>Entity colors</h3>
        <ul className={styles.kindList}>
          {allKinds.map((kind) => {
            const entry = KIND_ENTRIES[kind]
            return (
              <li key={kind} className={styles.kindItem}>
                <span
                  className={styles.swatch}
                  style={{ background: `var(--kind-${kind})` }}
                  aria-hidden
                />
                <span className={styles.kindLabel}>{entry.label}</span>
                <span className={styles.kindDesc}>{entry.description}</span>
              </li>
            )
          })}
        </ul>
      </section>
    </div>
  )
}
