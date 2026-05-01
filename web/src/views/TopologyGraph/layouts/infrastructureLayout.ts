import type { LayoutStrategy } from './types'
import { runDagreLayout, type DagreLayoutSpec } from './dagreLayout'

const DAGRE_SPEC: DagreLayoutSpec = {
  rankdir: 'LR',
  ranksep: 165,
  nodesep: 115,
  ranker: 'tight-tree',
  acyclicer: 'greedy',
  packNodeGroups: true,
}

export const infrastructureLayout: LayoutStrategy = {
  layout(input, deps) {
    return runDagreLayout(input, deps, DAGRE_SPEC)
  },
}
