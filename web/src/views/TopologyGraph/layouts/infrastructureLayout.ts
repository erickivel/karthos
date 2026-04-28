import type { LayoutStrategy } from './types'
import { runLayered } from './ownershipLayout'

const TOP_LEVEL_OPTIONS: Record<string, string> = {
  'elk.algorithm': 'layered',
  'elk.direction': 'RIGHT',
  'elk.spacing.nodeNode': '80',
}

const PARENT_OPTIONS: Record<string, string> = {
  'elk.algorithm': 'rectpacking',
  'elk.rectpacking.strategy': 'CURRENT_POSITION',
  'elk.spacing.nodeNode': '12',
  'elk.aspectRatio': '1.6',
}

export const infrastructureLayout: LayoutStrategy = {
  async layout(input, deps) {
    return runLayered(input, deps, TOP_LEVEL_OPTIONS, 'RIGHT', PARENT_OPTIONS)
  },
}
