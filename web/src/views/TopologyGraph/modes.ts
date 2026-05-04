import {
  Boxes,
  Server,
  Network,
  Eye,
  Activity,
  Layers,
  Workflow,
  Cpu,
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

export const GRANULARITY_ENTRIES: ModeEntry<Granularity>[] = [
  {
    value: 'overview',
    label: 'L1 — Overview',
    shortLabel: 'L1',
    icon: Eye,
    description: 'Namespaces or nodes, services, workloads, ingress.',
  },
  {
    value: 'runtime',
    label: 'L2 — Runtime',
    shortLabel: 'L2',
    icon: Activity,
    description: 'Adds pods to the overview level.',
  },
  {
    value: 'detailed',
    label: 'L3 — Detailed',
    shortLabel: 'L3',
    icon: Layers,
    description: 'Adds containers, configmaps, secrets and PVCs.',
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

export function granularityEntriesFor(
  mode: ProjectionMode,
): ModeEntry<Granularity>[] {
  return mode === 'traffic' ? TRAFFIC_GRANULARITY_ENTRIES : GRANULARITY_ENTRIES
}
