import type { ProjectionMode } from '../projection'
import type { LayoutStrategy } from './types'
import { ownershipLayout } from './ownershipLayout'
import { infrastructureLayout } from './infrastructureLayout'
import { trafficElkLayout } from './elkLayout'

export type LayoutAlgorithm = 'dagre' | 'tree' | 'elk'

export interface LayoutModeConfig {
  defaultAlgorithm: LayoutAlgorithm
  strategies: Partial<Record<LayoutAlgorithm, LayoutStrategy>>
}

export const LAYOUT_REGISTRY: Record<ProjectionMode, LayoutModeConfig> = {
  ownership: {
    defaultAlgorithm: 'tree',
    strategies: { tree: ownershipLayout },
  },
  infrastructure: {
    defaultAlgorithm: 'dagre',
    strategies: { dagre: infrastructureLayout },
  },
  traffic: {
    defaultAlgorithm: 'elk',
    strategies: { elk: trafficElkLayout },
  },
}

export interface ResolvedStrategy {
  algorithm: LayoutAlgorithm
  strategy: LayoutStrategy
}

export function resolveStrategy(
  mode: ProjectionMode,
  requested?: LayoutAlgorithm,
): ResolvedStrategy {
  const config = LAYOUT_REGISTRY[mode]
  const algorithm =
    requested && config.strategies[requested] ? requested : config.defaultAlgorithm
  const strategy = config.strategies[algorithm] ?? config.strategies[config.defaultAlgorithm]
  if (!strategy) {
    throw new Error(`No layout strategy registered for mode "${mode}"`)
  }
  return { algorithm, strategy }
}

export function availableAlgorithms(mode: ProjectionMode): LayoutAlgorithm[] {
  return Object.keys(LAYOUT_REGISTRY[mode].strategies) as LayoutAlgorithm[]
}
