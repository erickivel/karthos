import { createElement } from 'react'
import { renderToStaticMarkup } from 'react-dom/server'
import { describe, expect, it } from 'vitest'
import type { MetricSample } from '@/types/metrics'
import { RawSamplesTable } from './RawSamplesTable'

describe('Raw Samples table', () => {
  it('shows a short series identity with an accessible full-label disclosure', () => {
    const rows: MetricSample[] = [{
      metric_name: 'k8s.volume.capacity', entity_id: 'pod-1', value: 1024,
      timestamp: '2026-10-11T12:00:00Z', unit: 'bytes',
      labels: { source: 'kubelet', volume_name: 'config-volume', volume_type: 'emptyDir' },
    }]
    const html = renderToStaticMarkup(createElement(RawSamplesTable, { samples: rows }))
    expect(html).toContain('Series')
    expect(html).toContain('<details')
    expect(html).toContain('volume_name=config-volume')
    expect(html).toContain('volume_type=emptyDir')
    expect(html).toContain('Value label')
  })
})
