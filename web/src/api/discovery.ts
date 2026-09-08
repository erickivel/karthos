import { request } from './client'
import { capabilityProfileSchema, systemStatusSchema } from './schemas'
import type { CapabilityProfile, SystemStatus } from './schemas'

export function getStatus(): Promise<SystemStatus> {
  return request('/api/v1/status', systemStatusSchema).then(({ data }) => data)
}

export function getDiscovery(): Promise<CapabilityProfile> {
  return request('/api/v1/discovery', capabilityProfileSchema).then(({ data }) => data)
}
