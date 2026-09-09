export type PodMetricSection = 'overview' | 'health' | 'volumes'

export const POD_METRIC_SECTIONS: Array<{ id: PodMetricSection; label: string }> = [
  { id: 'overview', label: 'Overview' },
  { id: 'health', label: 'Health' },
  { id: 'volumes', label: 'Volumes' },
]

const SECTION_METRICS: Record<PodMetricSection, ReadonlySet<string>> = {
  overview: new Set(['k8s.pod.cpu.usage', 'k8s.pod.memory.working_set']),
  health: new Set(['k8s.pod.phase', 'k8s.pod.status_reason']),
  volumes: new Set([
    'k8s.volume.available',
    'k8s.volume.capacity',
    'k8s.volume.inodes.free',
    'k8s.volume.inodes.used',
    'k8s.volume.inodes',
  ]),
}

/** Keeps each Pod section bounded, so costly per-volume series remain deferred. */
export function metricsForPodSection(metricNames: readonly string[], section: PodMetricSection): string[] {
  const supported = SECTION_METRICS[section]
  return metricNames.filter((name) => supported.has(name))
}
