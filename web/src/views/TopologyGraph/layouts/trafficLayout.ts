import type { LayoutStrategy } from './types'
import { runLayered } from './ownershipLayout'

const ELK_OPTIONS: Record<string, string> = {
  'elk.algorithm': 'layered',
  'elk.direction': 'RIGHT',
  'elk.layered.cycleBreaking.strategy': 'GREEDY',
  'elk.layered.spacing.nodeNodeBetweenLayers': '90',
  'elk.spacing.nodeNode': '50',
  'elk.layered.crossingMinimization.strategy': 'LAYER_SWEEP',
  'elk.layered.nodePlacement.strategy': 'BRANDES_KOEPF',
  'elk.edgeRouting': 'ORTHOGONAL',
}

export const trafficLayout: LayoutStrategy = {
  async layout(input, deps) {
    return runLayered(input, deps, ELK_OPTIONS, 'RIGHT')
  },
}
