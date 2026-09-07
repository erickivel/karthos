import type { ProjectionMode } from '../projection'
import type { LayoutStrategy } from './types'
import { ownershipLayout } from './ownershipLayout'
import { infrastructureLayout } from './infrastructureLayout'
import { trafficElkLayout } from './elkLayout'

export const LAYOUT_REGISTRY: Record<ProjectionMode, LayoutStrategy> = {
  ownership: ownershipLayout,
  infrastructure: infrastructureLayout,
  traffic: trafficElkLayout,
}

export function resolveStrategy(mode: ProjectionMode): LayoutStrategy {
  const strategy = LAYOUT_REGISTRY[mode]
  if (!strategy) {
    throw new Error(`No layout strategy registered for mode "${mode}"`)
  }
  return strategy
}
