import type { EntityKind } from '@/types/graph'
import type { MetricCategory } from '@/types/metrics'

/**
 * Coarse-grained map of which metric *categories* (Explorer tabs) make
 * structural sense for each entity kind.
 *
 * Drives **tab visibility** in the Metrics Explorer. A category not listed
 * here will never produce a tab for that kind, regardless of which metrics
 * the catalog ships.
 *
 * Keep edits to this file mechanical: add a row, remove a row. No logic.
 *
 * See `./README.md` for the conceptual model.
 */
export const KIND_CATEGORIES: Record<EntityKind, MetricCategory[]> = {
  cluster: [],
  node: ['resource', 'capacity', 'health'],
  namespace: ['health'],
  workload: ['capacity', 'health'],
  pod: ['resource', 'traffic', 'latency', 'error', 'health'],
  container: ['resource', 'capacity'],
  service: ['traffic', 'latency', 'error'],
  ingress: ['traffic', 'latency', 'error'],
  endpoint: ['traffic', 'latency', 'error'],
  configmap: [],
  secret: [],
  pvc: ['capacity', 'resource'],
}
