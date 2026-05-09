import type { EntityKind } from '@/types/graph'

/**
 * Fine-grained map of which entity kinds each metric source emits data for.
 *
 * Drives **card visibility** within a visible tab: a card is shown if at
 * least one of the metric's catalog `sources` maps to the selected entity
 * kind here (or to the `'*'` wildcard).
 *
 * Source IDs are the values declared in `pkg/core/metrics/canonical.go`
 * (`SourceKubelet`, `SourceMeshIstio`, …) and surfaced in
 * `CanonicalMetric.sources[].id`.
 *
 * Use `'*'` for sources whose target kinds aren't statically classifiable
 * (catch-all exporters, custom Prometheus jobs, etc.). Wildcard sources are
 * applicable to every kind.
 *
 * Unknown source IDs (i.e. not present in this map) are treated as
 * applicable in production and trigger a `console.warn` in dev — see
 * `./applicability.ts`.
 */
export const SOURCE_KINDS: Record<string, EntityKind[] | '*'> = {
  kubelet: ['pod', 'container', 'pvc'],
  'host-os': ['node'],
  'kube-cluster': ['namespace', 'node', 'workload', 'container'],
  'prometheus.app': ['pod', 'service', 'workload'],
  'mesh.istio': ['service'],
  'traces.spanmetrics': ['service', 'pod'],
  'traces.servicegraph': ['service'],
}
