import { describe, expect, it } from 'vitest'
import { metricDetailRows, metricPresentation } from './metricPresentation'

describe('service metric presentation', () => {
  it('names Container resource and health details in plain language', () => {
    expect(metricPresentation({ canonicalName: 'k8s.container.cpu.request', scope: 'entity' })).toMatchObject({
      title: 'CPU request',
      meaning: 'CPU Kubernetes reserves for this Container when scheduling.',
    })
    expect(metricPresentation({ canonicalName: 'k8s.container.memory.limit', scope: 'entity' })).toMatchObject({
      title: 'Memory limit',
      meaning: 'Maximum memory Kubernetes allows this Container to use.',
    })
    expect(metricPresentation({ canonicalName: 'k8s.container.restarts', scope: 'entity' })).toMatchObject({
      title: 'Restart count',
      meaning: 'Cumulative number of times Kubernetes restarted this Container.',
    })
  })

  it('names direct Pod resources without leaking canonical names into the primary UI', () => {
    expect(metricPresentation({ canonicalName: 'k8s.pod.cpu.usage', scope: 'entity' })).toMatchObject({
      title: 'CPU used',
      meaning: 'Direct CPU use for this Pod, reported by kubelet.',
    })
    expect(metricPresentation({ canonicalName: 'k8s.pod.memory.working_set', scope: 'entity' })).toMatchObject({
      title: 'Working memory',
      meaning: 'Memory actively used by this Pod, reported by kubelet.',
    })
  })

  it('names an Istio 5xx edge series as a server-error request rate', () => {
    expect(metricPresentation({
      canonicalName: 'servicegraph.request.count',
      scope: 'edge',
      labels: { 'http.response.status_code': '500' },
    })).toMatchObject({
      title: 'Server error request rate',
      meaning: 'Requests on this connection that ended with a 5xx server error, per second.',
    })
  })

  it('keeps raw Prometheus HTTP status visible for service entity series', () => {
    expect(metricPresentation({
      canonicalName: 'http.server.request.count',
      scope: 'entity',
      labels: { status: '500' },
    }).seriesLabels).toContainEqual({ label: 'HTTP status', value: '500' })
    expect(metricPresentation({
      canonicalName: 'http.server.request.count',
      scope: 'entity',
      labels: { status: '200' },
    }).seriesLabels).toContainEqual({ label: 'HTTP status', value: '200' })
  })

  it('names a servicegraph entity metric as outbound traffic', () => {
    expect(metricPresentation({
      canonicalName: 'servicegraph.request.count',
      scope: 'entity',
      labels: {},
    })).toMatchObject({
      title: 'Outbound request rate',
      meaning: 'Requests this service sends to other services per second.',
    })
  })

  it('makes the destination explicit for an outbound service series', () => {
    expect(metricPresentation({
      canonicalName: 'servicegraph.request.count',
      scope: 'entity',
      labels: {
        edge_server: 'service-b-node',
        'http.response.status_code': '500',
      },
    })).toMatchObject({
      title: 'Outbound request rate · to service-b-node',
      seriesLabels: [
        { label: 'Destination', value: 'service-b-node' },
        { label: 'HTTP status', value: '500' },
      ],
    })
  })

  it('does not repeat error in a servicegraph error-series title', () => {
    expect(metricPresentation({
      canonicalName: 'servicegraph.request.error.count',
      scope: 'edge',
      labels: { 'http.response.status_code': '500' },
    })).toMatchObject({
      title: 'Server error rate',
    })
  })

  it('exposes only the labels which make this series distinct in metric details', () => {
    expect(metricPresentation({
      canonicalName: 'servicegraph.request.duration',
      scope: 'edge',
      labels: {
        'http.response.status_code': '200',
        'network.protocol.name': 'http',
        'mesh.security.policy': 'mutual_tls',
        source: 'mesh.istio',
        edge_client_id: 'client-id',
        edge_server_id: 'server-id',
      },
    }).seriesLabels).toEqual([
      { label: 'HTTP status', value: '200' },
      { label: 'Protocol', value: 'HTTP' },
      { label: 'Transport security', value: 'mTLS' },
    ])
  })

  it('builds metric details from the effective query and observed sample', () => {
    expect(metricDetailRows({
      meaning: 'Requests on this connection that ended with a 5xx server error, per second.',
      observedSource: 'mesh.istio',
      query: {
        aggregation: 'window_rate',
        window: '1m',
        from: '2026-09-05T22:34:25Z',
        to: '2026-09-05T22:49:25Z',
        step: '15s',
        timestamp_semantics: 'evaluation',
      },
      seriesLabels: [{ label: 'HTTP status', value: '500' }],
    })).toEqual([
      { label: 'What this shows', value: 'Requests on this connection that ended with a 5xx server error, per second.' },
      { label: 'Observed source', value: 'mesh.istio' },
      { label: 'Query', value: 'window rate · 1m window · 15s grid' },
      { label: 'Series labels', value: 'HTTP status: 500' },
    ])
  })
})
