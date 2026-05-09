export type SystemStatusValue = 'nominal' | 'degraded' | 'disconnected'
export type ComponentStatus = 'available' | 'unavailable'

export interface HealthSummary {
  nodes: {
    healthy: number
    unhealthy: number
  }
  pods: {
    healthy: number
    running_not_ready: number
    restarting: number
    pending: number
    succeeded: number
    failed: number
    unknown: number
  }
}

export interface SystemStatus {
  system_status: SystemStatusValue
  reasons: string[]
  version: { major: number; minor: number }
  components: {
    metric_store: ComponentStatus
    graph_store: ComponentStatus
    metadata_store: ComponentStatus
    entity_registry: ComponentStatus
  }
  uptime_seconds: number
  last_snapshot_at: string
  last_discovery_at: string
  snapshot_freshness_seconds: number
  data_stale: boolean
  health_summary?: HealthSummary
}

export interface DiscoverySource {
  type: string
  name: string
}

export interface CollectionPath {
  source: string
  receiver: string
  pipeline: string
}

export interface DiscoverySummary {
  cluster_id: string
  cluster_name: string
  discovered_at: string
  node_count: number
  namespace_count: number
  workload_count: number
  service_count: number
  ingress_count: number
  sources: DiscoverySource[]
  collection_paths: CollectionPath[]
  recommended_receivers: string[]
}
