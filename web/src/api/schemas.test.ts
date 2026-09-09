import { describe, expect, it } from 'vitest'
import { capabilityProfileSchema, metricCatalogResponseSchema, metricSeriesResponseSchema, systemStatusSchema } from './schemas'

describe('API response schemas', () => {
  it('accepts the current capability profile contract', () => {
    const profile = capabilityProfileSchema.parse({
      cluster_id: 'cluster:demo',
      cluster_name: 'demo',
      discovered_at: '2026-09-04T12:00:00Z',
      capabilities: [
        {
          source_id: 'kubelet',
          kind: 'direct',
          status: 'available',
          signals: ['metrics'],
          evidence: {},
        },
        {
          source_id: 'prometheus.app',
          kind: 'direct',
          status: 'unavailable',
          signals: null,
          evidence: {},
        },
      ],
    })

    expect(profile.capabilities[0]).toMatchObject({
      source_id: 'kubelet',
      kind: 'direct',
      status: 'available',
    })
    expect(profile.capabilities[1]?.signals).toEqual([])
  })

  it('rejects a malformed capability profile before a view consumes it', () => {
    expect(capabilityProfileSchema.safeParse({
      cluster_name: 'demo',
      capabilities: [],
    }).success).toBe(false)
  })

  it('accepts catalog metrics whose non-applicable aggregation list is null', () => {
    const catalog = metricCatalogResponseSchema.parse({
      metrics: [
        {
          name: 'http.server.request.duration',
          display_name: 'HTTP Server Request Duration',
          unit: 'milliseconds',
          category: 'latency',
          scopes: ['edge'],
          description: 'Duration of server requests.',
          sources: [{ id: 'prometheus.app', priority: 1 }],
          aggregation: {
            supported: null,
            default: 'window_mean',
            default_window: '1m',
            base_unit: 'nanoseconds',
            rate_unit: 'nanoseconds',
          },
        },
      ],
      total: 1,
    })

    expect(catalog.metrics[0]?.aggregation?.supported).toEqual([])
  })

  it('accepts a nominal status response with fractional snapshot timestamps', () => {
    const status = systemStatusSchema.parse({
      components: {
        entity_registry: 'available',
        graph_store: 'available',
        metadata_store: 'available',
        metric_store: 'available',
      },
      data_stale: false,
      health_summary: {
        status: 'complete',
        nodes: { status: 'complete', healthy: 0, unhealthy: 0, unknown: 1 },
        pods: { status: 'complete', healthy: 18, pending: 0, succeeded: 2, failed: 0, unknown: 111 },
      },
      last_discovery_at: '2026-09-04T14:31:04.436969Z',
      last_snapshot_at: '2026-09-04T20:51:22.53854Z',
      reasons: [],
      snapshot_freshness_seconds: 275.202338749,
      system_status: 'nominal',
      uptime_seconds: 23075,
      version: { major: 1, minor: 0 },
    })

    expect(status.system_status).toBe('nominal')
  })

  it('preserves series labels, edge IDs, enum labels, and query metadata', () => {
    const response = metricSeriesResponseSchema.parse({
      series: [
        {
          metric_name: 'servicegraph.request.count',
          unit: 'per_second',
          category: 'traffic',
          description: 'Request rate',
          edge: { source_id: 'service:a', target_id: 'service:b' },
          samples: [
            {
              metric_name: 'servicegraph.request.count',
              entity_id: 'service:a',
              value: 3.5,
              value_label: 'healthy',
              timestamp: '2026-09-04T12:00:00Z',
              unit: 'per_second',
              labels: { source: 'traces.servicegraph', failed: 'false' },
              edge: { source_id: 'service:a', target_id: 'service:b' },
            },
          ],
        },
      ],
      total: 1,
      query: {
        aggregation: 'window_rate',
        window: '1m',
        from: '2026-09-04T11:45:00Z',
        to: '2026-09-04T12:00:00Z',
        step: '15s',
        timestamp_semantics: 'evaluation',
      },
    })

    expect(response.series[0]?.samples[0]?.value_label).toBe('healthy')
    expect(response.query.timestamp_semantics).toBe('evaluation')
  })
})
