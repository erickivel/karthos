import { request } from './client'
import type { SystemStatus, DiscoverySummary } from '@/types/status'

export function getStatus(): Promise<SystemStatus> {
  return request<SystemStatus>('/api/v1/status')
}

export function getDiscovery(): Promise<DiscoverySummary> {
  return request<DiscoverySummary>('/api/v1/discovery')
}
