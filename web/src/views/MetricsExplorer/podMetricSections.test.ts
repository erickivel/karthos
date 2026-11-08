import { describe, expect, it } from 'vitest'
import { metricsForPodSection, POD_METRIC_SECTIONS } from './podMetricSections'

const podMetricNames = [
  'k8s.pod.cpu.usage',
  'k8s.pod.memory.working_set',
  'k8s.pod.phase',
  'k8s.pod.status_reason',
  'k8s.pod.volume.available',
  'k8s.pod.volume.capacity',
  'k8s.pod.volume.inodes.free',
  'k8s.pod.volume.inodes.used',
  'k8s.pod.volume.inodes',
]

describe('Pod metric sections', () => {
  it('keeps the initial Overview request limited to direct CPU and memory', () => {
    expect(metricsForPodSection(podMetricNames, 'overview')).toEqual([
      'k8s.pod.cpu.usage',
      'k8s.pod.memory.working_set',
    ])
  })

  it('defers enum diagnostics and volume families to their dedicated sections', () => {
    expect(POD_METRIC_SECTIONS.map((section) => section.label)).toEqual(['Overview', 'Health', 'Volumes'])
    expect(metricsForPodSection(podMetricNames, 'health')).toEqual([
      'k8s.pod.phase',
      'k8s.pod.status_reason',
    ])
    expect(metricsForPodSection(podMetricNames, 'volumes')).toEqual([
      'k8s.pod.volume.available',
      'k8s.pod.volume.capacity',
      'k8s.pod.volume.inodes.free',
      'k8s.pod.volume.inodes.used',
      'k8s.pod.volume.inodes',
    ])
  })
})
