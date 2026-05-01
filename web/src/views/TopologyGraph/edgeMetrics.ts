import type { RelationType } from '@/types/graph'

export interface EdgeMetricSnapshot {
  requestRate?: number
  latencyNs?: number
  errorRate?: number
  errorRatio?: number
  source?: string
  mtls?: boolean
  protocol?: string
  peerSource?: string
  peerDestination?: string
}

export interface GraphEdgeMetricData {
  sourceName?: string
  targetName?: string
  metrics?: EdgeMetricSnapshot
}

export function buildEdgeMetricKey(client: string, server: string): string {
  return `${client}:::${server}`.toLowerCase()
}

export function getEdgeMetricKeyFromLabels(labels?: Record<string, string>): string | null {
  const client = labels?.edge_client?.trim()
  const server = labels?.edge_server?.trim()

  if (!client || !server) return null
  return buildEdgeMetricKey(client, server)
}

export function getEdgeMetricKeyFromNodes(
  relation: RelationType,
  sourceName?: string,
  targetName?: string,
): string | null {
  if (relation !== 'calls' || !sourceName || !targetName) return null
  return buildEdgeMetricKey(sourceName, targetName)
}
