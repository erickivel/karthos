import { z } from 'zod'

const entityKindValues = [
  'cluster', 'node', 'namespace', 'workload', 'pod', 'container',
  'service', 'ingress', 'endpoint', 'configmap', 'secret', 'pvc', 'hpa',
] as const

const relationValues = [
  'calls', 'runs_on', 'contains', 'routes_to', 'exposes', 'uses', 'mounts',
] as const

const metricCategoryValues = [
  'resource', 'traffic', 'latency', 'error', 'capacity', 'health',
] as const

const metricScopeValues = ['entity', 'edge'] as const
const aggregationValues = ['latest', 'window_rate', 'active_rate', 'increase', 'window_mean'] as const

export const entityKindSchema = z.enum(entityKindValues)
export const relationSchema = z.enum(relationValues)
export const metricCategorySchema = z.enum(metricCategoryValues)
export const metricScopeSchema = z.enum(metricScopeValues)
export const timestampSchema = z.string().datetime({ offset: true })
export const labelsSchema = z.record(z.string(), z.string())

export const sourceCapabilitySchema = z.object({
  source_id: z.string().min(1),
  kind: z.enum(['direct', 'derived']),
  status: z.enum(['available', 'partial', 'unavailable']),
  // The backend serializes unavailable modules with a null signal list. The
  // frontend treats that as the same semantic value as no supported signals.
  signals: z.array(z.string()).nullable().transform((signals) => signals ?? []),
  evidence: z.object({
    prometheus_targets: z.array(z.object({
      service_name: z.string(),
      namespace: z.string(),
      port: z.number().int(),
      path: z.string(),
    })).optional(),
    otlp: z.object({ traces_enabled: z.boolean() }).optional(),
    mesh: z.object({
      flavor: z.string(),
      mode: z.string(),
      injected_namespaces: z.array(z.string()).optional(),
      metrics_port: z.number().int().optional(),
      metrics_path: z.string().optional(),
    }).optional(),
  }).optional(),
})

export const capabilityProfileSchema = z.object({
  cluster_id: z.string().min(1),
  cluster_name: z.string().min(1),
  discovered_at: timestampSchema,
  capabilities: z.array(sourceCapabilitySchema),
})

const componentStatusSchema = z.enum(['available', 'unavailable'])
const healthQualitySchema = z.enum(['complete', 'partial', 'unavailable'])

export const healthSummarySchema = z.object({
  status: healthQualitySchema,
  nodes: z.object({
    status: healthQualitySchema,
    healthy: z.number().int().nonnegative(),
    unhealthy: z.number().int().nonnegative(),
    unknown: z.number().int().nonnegative(),
  }),
  pods: z.object({
    status: healthQualitySchema,
    healthy: z.number().int().nonnegative(),
    pending: z.number().int().nonnegative(),
    succeeded: z.number().int().nonnegative(),
    failed: z.number().int().nonnegative(),
    unknown: z.number().int().nonnegative(),
  }),
})

export const systemStatusSchema = z.object({
  system_status: z.enum(['nominal', 'degraded', 'disconnected']),
  reasons: z.array(z.string()),
  version: z.object({ major: z.number().int(), minor: z.number().int() }),
  components: z.object({
    metric_store: componentStatusSchema,
    graph_store: componentStatusSchema,
    metadata_store: componentStatusSchema,
    entity_registry: componentStatusSchema,
  }),
  uptime_seconds: z.number().int().nonnegative(),
  last_snapshot_at: timestampSchema.optional(),
  last_discovery_at: timestampSchema.optional(),
  snapshot_freshness_seconds: z.number().nonnegative(),
  data_stale: z.boolean(),
  health_summary: healthSummarySchema.optional(),
})

export const graphNodeSchema = z.object({
  id: z.string().min(1),
  kind: entityKindSchema,
  name: z.string().default(''),
  namespace: z.string().default(''),
  cluster: z.string().default(''),
  status: z.string(),
  labels: labelsSchema.optional(),
})

export const graphEdgeSchema = z.object({
  id: z.string().min(1),
  source: z.string().min(1),
  target: z.string().min(1),
  relation: relationSchema,
  confidence: z.number(),
  provenance: z.array(z.string()).optional(),
  first_seen: timestampSchema,
  last_seen: timestampSchema,
  metadata: labelsSchema.optional(),
})

const healthEnrichmentSchema = z.object({
  status: healthQualitySchema,
  pods: healthQualitySchema,
  containers: healthQualitySchema,
})

export const materializedGraphSnapshotSchema = z.object({
  id: z.number().int().positive(),
  created_at: timestampSchema,
})

export const graphSnapshotSchema = z.object({
	 snapshot: materializedGraphSnapshotSchema,
  version: z.object({ major: z.number().int(), minor: z.number().int() }),
  from: timestampSchema,
  to: timestampSchema,
  created_at: timestampSchema,
  health_enrichment: healthEnrichmentSchema.optional(),
  graph: z.object({
    timestamp: timestampSchema,
    nodes: z.array(graphNodeSchema),
    edges: z.array(graphEdgeSchema),
  }),
  summary: z.object({
    node_count: z.number().int().nonnegative(),
    edge_count: z.number().int().nonnegative(),
    kind_counts: z.record(z.string(), z.number().int().nonnegative()),
    relation_counts: z.record(z.string(), z.number().int().nonnegative()),
    snapshot_age_seconds: z.number().nonnegative(),
    available_filters: z.object({
      kinds: z.array(entityKindSchema),
      namespaces: z.array(z.string()),
      relations: z.array(relationSchema),
    }),
  }),
})

export const metricEdgeSchema = z.object({
  source_id: z.string().min(1),
  target_id: z.string().min(1),
})

export const metricSampleSchema = z.object({
  metric_name: z.string().min(1),
  entity_id: z.string().min(1),
  value: z.number(),
  value_label: z.string().optional(),
  timestamp: timestampSchema,
  unit: z.string(),
  labels: labelsSchema.optional(),
  edge: metricEdgeSchema.optional(),
})

export const metricQuerySchema = z.object({
  aggregation: z.enum(aggregationValues),
  window: z.string().optional(),
  from: timestampSchema,
  to: timestampSchema,
  step: z.string().min(1),
  timestamp_semantics: z.literal('evaluation'),
})

export const metricSeriesSchema = z.object({
  metric_name: z.string().min(1),
  unit: z.string(),
  category: metricCategorySchema,
  description: z.string(),
  samples: z.array(metricSampleSchema),
  edge: metricEdgeSchema.optional(),
})

const aggregationSpecSchema = z.object({
  supported: z.array(z.enum(aggregationValues)).nullable().transform((supported) => supported ?? []),
  default: z.enum(aggregationValues),
  default_window: z.string().optional(),
  base_unit: z.string(),
  rate_unit: z.string(),
})

export const canonicalMetricSchema = z.object({
  name: z.string().min(1),
  display_name: z.string(),
  unit: z.string(),
  category: metricCategorySchema,
  scopes: z.array(metricScopeSchema),
  applies_to: z.array(entityKindSchema).optional(),
  dimensions: z.array(z.string()).optional(),
  description: z.string(),
  sources: z.array(z.object({ id: z.string().min(1), priority: z.number().int() })).min(1),
  aggregation: aggregationSpecSchema.optional(),
})

export const metricSeriesResponseSchema = z.object({
  query: metricQuerySchema,
  series: z.array(metricSeriesSchema),
  total: z.number().int().nonnegative(),
  next_token: z.string().optional(),
})

export const metricCatalogResponseSchema = z.object({
  metrics: z.array(canonicalMetricSchema),
  total: z.number().int().nonnegative(),
})

export const catalogStatsResponseSchema = z.object({
  lookback_seconds: z.number().int().nonnegative(),
  stats: z.array(z.object({
    metric_name: z.string().min(1),
    by_source: z.array(z.object({
      source_id: z.string().min(1),
      priority: z.number().int().optional(),
      sample_count: z.number().int().nonnegative(),
      last_seen_at: timestampSchema.nullable(),
    })),
  })),
})

export const apiErrorSchema = z.object({
  error: z.string(),
  code: z.number().int(),
})

export const apiEnvelopeSchema = <T extends z.ZodType>(data: T) => z.object({
  data,
  next_token: z.string().optional(),
})

export type CapabilityProfile = z.infer<typeof capabilityProfileSchema>
export type SystemStatus = z.infer<typeof systemStatusSchema>
export type HealthSummary = z.infer<typeof healthSummarySchema>
export type GraphSnapshot = z.infer<typeof graphSnapshotSchema>
export type GraphNode = z.infer<typeof graphNodeSchema>
export type GraphEdge = z.infer<typeof graphEdgeSchema>
export type MetricSample = z.infer<typeof metricSampleSchema>
export type MetricSeries = z.infer<typeof metricSeriesSchema>
export type CanonicalMetric = z.infer<typeof canonicalMetricSchema>
export type MetricSeriesResponse = z.infer<typeof metricSeriesResponseSchema>
export type MetricCatalogResponse = z.infer<typeof metricCatalogResponseSchema>
export type CatalogStatsResponse = z.infer<typeof catalogStatsResponseSchema>
export type EntityKind = z.infer<typeof entityKindSchema>
export type RelationType = z.infer<typeof relationSchema>
export type MetricCategory = z.infer<typeof metricCategorySchema>
export type MetricScope = z.infer<typeof metricScopeSchema>
