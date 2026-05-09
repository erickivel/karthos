import {
  Boxes,
  Server,
  Network,
  Workflow,
  Cpu,
  Box,
  Container,
  Layers,
  type LucideIcon,
} from 'lucide-react'
import type { Granularity, ProjectionMode } from './projection'

export interface ModeEntry<T extends string> {
  value: T
  label: string
  shortLabel: string
  icon: LucideIcon
  description: string
}

export const MODE_ENTRIES: ModeEntry<ProjectionMode>[] = [
  {
    value: 'ownership',
    label: 'Ownership',
    shortLabel: 'Ownership',
    icon: Boxes,
    description: 'Group entities by namespace — shows who owns what.',
  },
  {
    value: 'infrastructure',
    label: 'Infrastructure',
    shortLabel: 'Infra',
    icon: Server,
    description: 'Group pods/containers by their host node — shows where things run.',
  },
  {
    value: 'traffic',
    label: 'Traffic',
    shortLabel: 'Traffic',
    icon: Network,
    description: 'Service-centric view with no parent grouping — focus on edges.',
  },
]

// Ownership mode reuses the Granularity URL state but shows the K8s object
// hierarchy in human terms instead of L1/L2/L3. URL values stay
// overview/runtime/detailed for shareable-link stability — same convention as
// Traffic and Infrastructure.
export const OWNERSHIP_GRANULARITY_ENTRIES: ModeEntry<Granularity>[] = [
  {
    value: 'overview',
    label: 'Workloads & services',
    shortLabel: 'Workloads',
    icon: Workflow,
    description: 'Top-level objects per namespace: services, workloads, ingress.',
  },
  {
    value: 'runtime',
    label: 'Pods per workload',
    shortLabel: 'Pods',
    icon: Box,
    description: 'Adds pod replicas under each workload.',
  },
  {
    value: 'detailed',
    label: 'Containers & resources',
    shortLabel: 'Resources',
    icon: Layers,
    description: 'Adds containers, configmaps, secrets, and PVCs to the tree.',
  },
]

// Traffic mode reuses the Granularity URL state but presents three named
// flows instead of L1/L2/L3. Keeping the underlying value IDs unchanged
// preserves shareable links across modes.
export const TRAFFIC_GRANULARITY_ENTRIES: ModeEntry<Granularity>[] = [
  {
    value: 'overview',
    label: 'Endpoint Flow',
    shortLabel: 'Endpoint',
    icon: Network,
    description:
      'Logical traffic surface — ingress/gateway and services. Routes and call edges only.',
  },
  {
    value: 'runtime',
    label: 'Workload Flow',
    shortLabel: 'Workload',
    icon: Workflow,
    description:
      'Which controllers implement traffic — services, workloads, ingress. Exposes is structural.',
  },
  {
    value: 'detailed',
    label: 'Instance Flow',
    shortLabel: 'Instance',
    icon: Cpu,
    description:
      'Pod replicas participating in traffic. Pod-level call edges only.',
  },
]

// Infrastructure mode reuses the Granularity URL state but presents three
// physically meaningful tiers — Nodes / Pods / Containers — instead of
// L1/L2/L3. URL values stay overview/runtime/detailed for shareable links.
export const INFRASTRUCTURE_GRANULARITY_ENTRIES: ModeEntry<Granularity>[] = [
  {
    value: 'overview',
    label: 'Nodes',
    shortLabel: 'Nodes',
    icon: Server,
    description: 'Just the cluster nodes — where compute lives.',
  },
  {
    value: 'runtime',
    label: 'Pods on nodes',
    shortLabel: 'Pods',
    icon: Box,
    description: 'Pods nested inside the node that hosts them.',
  },
  {
    value: 'detailed',
    label: 'Containers in pods',
    shortLabel: 'Containers',
    icon: Container,
    description: 'Full Node ⊃ Pod ⊃ Container nesting.',
  },
]

export function granularityEntriesFor(
  mode: ProjectionMode,
): ModeEntry<Granularity>[] {
  if (mode === 'traffic') return TRAFFIC_GRANULARITY_ENTRIES
  if (mode === 'infrastructure') return INFRASTRUCTURE_GRANULARITY_ENTRIES
  return OWNERSHIP_GRANULARITY_ENTRIES
}
