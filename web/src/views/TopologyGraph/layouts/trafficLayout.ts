import { MarkerType, Position, type Edge, type Node } from '@xyflow/react'
import type { GraphEdge, GraphNode, RelationType } from '@/types/graph'
import {
  NODE_HEIGHT_DEFAULT,
  NODE_HEIGHT_SMALL,
  NODE_WIDTH,
  SMALL_KINDS,
  type LayoutDeps,
  type LayoutInput,
  type LayoutResult,
  type LayoutStrategy,
} from './types'
import { getEdgeMetricKeyFromNodes } from '../edgeMetrics'
import { findSccs } from './sccCondensation'

const DIRECTED_RELATIONS: RelationType[] = ['calls', 'runs_on', 'routes_to', 'exposes']

const RANK_GAP_X = 220 // horizontal gap between depth columns
const SIBLING_GAP_Y = 50 // vertical gap between siblings under the same parent
const ROOT_GAP_Y = 80 // vertical gap between top-level subtrees

const SCC_NODE_GAP = 28
const SCC_PADDING = 24

interface SccGeometry {
  radius: number
  width: number
  height: number
  offsets: Map<string, { x: number; y: number }>
}

interface SccInfo {
  superId: string
  members: GraphNode[]
  geometry: SccGeometry | null
  width: number
  height: number
}

function nodeHeight(node: GraphNode): number {
  return SMALL_KINDS.includes(node.kind) ? NODE_HEIGHT_SMALL : NODE_HEIGHT_DEFAULT
}

function layoutSccCircle(members: GraphNode[]): SccGeometry {
  const n = members.length
  const memberHalfDiagonal = Math.hypot(NODE_WIDTH, NODE_HEIGHT_DEFAULT) / 2
  const angularRadius =
    n > 1
      ? (memberHalfDiagonal + SCC_NODE_GAP) / Math.sin(Math.PI / n)
      : 0
  const radius = Math.max(memberHalfDiagonal * 1.4, angularRadius)
  const offsets = new Map<string, { x: number; y: number }>()
  members.forEach((member, i) => {
    const angle = (i / n) * Math.PI * 2 - Math.PI / 2
    offsets.set(member.id, {
      x: Math.cos(angle) * radius,
      y: Math.sin(angle) * radius,
    })
  })
  const dim = (radius + memberHalfDiagonal + SCC_PADDING) * 2
  return { radius, width: dim, height: dim, offsets }
}

export const trafficLayout: LayoutStrategy = {
  layout(input: LayoutInput, deps: LayoutDeps): LayoutResult {
    return runTrafficLayout(input, deps)
  },
}

function runTrafficLayout(input: LayoutInput, deps: LayoutDeps): LayoutResult {
  const { nodes: graphNodes, edges: graphEdges } = input
  if (graphNodes.length === 0) return { nodes: [], edges: [] }

  const nodeMap = new Map(graphNodes.map((n) => [n.id, n]))

  // 1. SCCs over the directed traffic subgraph. Structural relations are
  //    excluded so they can't fabricate cycles.
  const directedEdges = graphEdges.filter((e) =>
    DIRECTED_RELATIONS.includes(e.relation),
  )
  const sccs = findSccs(
    graphNodes.map((n) => n.id),
    directedEdges.map((e) => ({ source: e.source, target: e.target })),
  )

  const sccInfos: SccInfo[] = sccs.map((memberIds, idx) => {
    const members = memberIds
      .map((id) => nodeMap.get(id))
      .filter((n): n is GraphNode => n !== undefined)
    const isMulti = members.length > 1
    const geometry = isMulti ? layoutSccCircle(members) : null
    return {
      superId: isMulti ? `scc:${idx}` : members[0]!.id,
      members,
      geometry,
      width: geometry ? geometry.width : NODE_WIDTH,
      height: geometry ? geometry.height : nodeHeight(members[0]!),
    }
  })

  const memberToSuper = new Map<string, string>()
  for (const info of sccInfos) {
    for (const m of info.members) memberToSuper.set(m.id, info.superId)
  }
  const infoBySuperId = new Map(sccInfos.map((info) => [info.superId, info]))

  // 2. Build the condensation DAG.
  const incoming = new Map<string, string[]>()
  const outgoing = new Map<string, string[]>()
  for (const info of sccInfos) {
    incoming.set(info.superId, [])
    outgoing.set(info.superId, [])
  }
  const seenCondEdge = new Set<string>()
  for (const edge of directedEdges) {
    const s = memberToSuper.get(edge.source)
    const t = memberToSuper.get(edge.target)
    if (!s || !t || s === t) continue
    const key = `${s}->${t}`
    if (seenCondEdge.has(key)) continue
    seenCondEdge.add(key)
    outgoing.get(s)!.push(t)
    incoming.get(t)!.push(s)
  }

  // 3. Rank assignment: longest path from any root through the DAG.
  const rankOf = new Map<string, number>()
  for (const info of sccInfos) rankOf.set(info.superId, 0)
  const sortedSupers = sccInfos.map((info) => info.superId).sort()
  const rankRoots = sortedSupers.filter(
    (id) => (incoming.get(id) ?? []).length === 0,
  )
  const rankVisit = (id: string, rank: number, stack: Set<string>) => {
    if (stack.has(id)) return
    stack.add(id)
    rankOf.set(id, Math.max(rankOf.get(id) ?? 0, rank))
    for (const next of outgoing.get(id) ?? []) {
      rankVisit(next, (rankOf.get(id) ?? rank) + 1, stack)
    }
    stack.delete(id)
  }
  for (const r of rankRoots) rankVisit(r, 0, new Set())

  // 4. Pick a single tree-parent per super-node: incoming source with the
  //    lowest rank (ties broken alphabetically). This turns the DAG into a
  //    spanning tree purely for placement; non-tree edges still draw on top.
  const treeParent = new Map<string, string | null>()
  for (const id of sortedSupers) {
    const ins = incoming.get(id) ?? []
    if (ins.length === 0) {
      treeParent.set(id, null)
      continue
    }
    let best: string | null = null
    let bestRank = Infinity
    for (const src of ins) {
      const r = rankOf.get(src) ?? 0
      if (r < bestRank || (r === bestRank && best !== null && src < best)) {
        bestRank = r
        best = src
      }
    }
    treeParent.set(id, best)
  }

  const treeChildren = new Map<string, string[]>()
  for (const id of sortedSupers) treeChildren.set(id, [])
  for (const id of sortedSupers) {
    const p = treeParent.get(id)
    if (p && p !== id) treeChildren.get(p)!.push(id)
  }
  for (const list of treeChildren.values()) list.sort()

  // 5. Compute per-rank x positions: x[r] = sum of widths of ranks 0..r-1
  //    plus gaps. Each rank's column width is the max super-node width at
  //    that rank, so columns line up cleanly even with different-sized SCCs.
  const rankWidths = new Map<number, number>()
  for (const info of sccInfos) {
    const r = rankOf.get(info.superId) ?? 0
    rankWidths.set(r, Math.max(rankWidths.get(r) ?? 0, info.width))
  }
  const sortedRanks = [...rankWidths.keys()].sort((a, b) => a - b)
  const rankCenterX = new Map<number, number>()
  let xCursor = 0
  for (const r of sortedRanks) {
    const w = rankWidths.get(r) ?? NODE_WIDTH
    rankCenterX.set(r, xCursor + w / 2)
    xCursor += w + RANK_GAP_X
  }

  // 6. Bottom-up subtree height for each super-node.
  const subtreeHeight = new Map<string, number>()
  const computeHeight = (id: string, stack: Set<string>): number => {
    const cached = subtreeHeight.get(id)
    if (cached !== undefined) return cached
    if (stack.has(id)) return infoBySuperId.get(id)!.height
    stack.add(id)
    const children = treeChildren.get(id) ?? []
    let total = 0
    children.forEach((c, i) => {
      total += computeHeight(c, stack)
      if (i < children.length - 1) total += SIBLING_GAP_Y
    })
    stack.delete(id)
    const own = infoBySuperId.get(id)!.height
    const h = Math.max(own, total)
    subtreeHeight.set(id, h)
    return h
  }
  const treeRoots = sortedSupers.filter((id) => treeParent.get(id) === null)
  for (const id of treeRoots) computeHeight(id, new Set())
  for (const id of sortedSupers) computeHeight(id, new Set())

  // 7. Top-down placement: each super-node sits at (rankCenterX[rank],
  //    topEdge + subtreeHeight/2). Children are stacked vertically and the
  //    block is centered against the parent's subtree slot.
  const centerOf = new Map<string, { cx: number; cy: number }>()
  const placed = new Set<string>()
  const place = (id: string, topEdge: number) => {
    if (placed.has(id)) return
    placed.add(id)
    const info = infoBySuperId.get(id)!
    const r = rankOf.get(id) ?? 0
    const subH = subtreeHeight.get(id) ?? info.height
    const cx = rankCenterX.get(r) ?? 0
    const cy = topEdge + subH / 2
    centerOf.set(id, { cx, cy })

    const children = (treeChildren.get(id) ?? []).filter(
      (c) => !placed.has(c),
    )
    if (children.length === 0) return

    let childTotal = 0
    children.forEach((c, i) => {
      childTotal += subtreeHeight.get(c) ?? infoBySuperId.get(c)!.height
      if (i < children.length - 1) childTotal += SIBLING_GAP_Y
    })
    let cursor = topEdge + (subH - childTotal) / 2
    for (const child of children) {
      place(child, cursor)
      cursor +=
        (subtreeHeight.get(child) ?? infoBySuperId.get(child)!.height) +
        SIBLING_GAP_Y
    }
  }

  let topCursor = 0
  for (const id of treeRoots) {
    place(id, topCursor)
    topCursor +=
      (subtreeHeight.get(id) ?? infoBySuperId.get(id)!.height) + ROOT_GAP_Y
  }
  for (const id of sortedSupers) {
    if (placed.has(id)) continue
    place(id, topCursor)
    topCursor +=
      (subtreeHeight.get(id) ?? infoBySuperId.get(id)!.height) + ROOT_GAP_Y
  }

  // 8. Emit positioned member nodes. Multi-member SCCs distribute around the
  //    super-node center using the precomputed circle offsets.
  const positioned: Node[] = []
  for (const info of sccInfos) {
    const center = centerOf.get(info.superId)
    if (!center) continue
    if (info.geometry) {
      for (const member of info.members) {
        const offset = info.geometry.offsets.get(member.id) ?? { x: 0, y: 0 }
        positioned.push(
          toReactNode(member, center.cx + offset.x, center.cy + offset.y, deps),
        )
      }
    } else {
      positioned.push(toReactNode(info.members[0]!, center.cx, center.cy, deps))
    }
  }

  const edges = graphEdges
    .filter((e) => nodeMap.has(e.source) && nodeMap.has(e.target))
    .map((edge) => toReactEdge(edge, nodeMap, deps))

  return { nodes: positioned, edges }
}

function toReactNode(
  node: GraphNode,
  centerX: number,
  centerY: number,
  deps: LayoutDeps,
): Node {
  const height = nodeHeight(node)
  return {
    id: node.id,
    type: 'entity',
    position: {
      x: centerX - NODE_WIDTH / 2,
      y: centerY - height / 2,
    },
    sourcePosition: Position.Right,
    targetPosition: Position.Left,
    data: {
      ...node,
      metrics: deps.nodeMetrics?.get(node.id) ?? {},
      mode: deps.mode,
    } as Record<string, unknown>,
  }
}

function toReactEdge(
  edge: GraphEdge,
  nodeMap: Map<string, GraphNode>,
  deps: LayoutDeps,
): Edge {
  const sourceNode = nodeMap.get(edge.source)
  const targetNode = nodeMap.get(edge.target)
  const edgeMetricKey = getEdgeMetricKeyFromNodes(
    edge.relation,
    sourceNode?.name,
    targetNode?.name,
  )
  return {
    id: edge.id,
    source: edge.source,
    target: edge.target,
    type: 'relation',
    zIndex: edge.relation === 'calls' ? 12 : 6,
    data: {
      ...edge,
      sourceName: sourceNode?.name,
      targetName: targetNode?.name,
      metrics: edgeMetricKey ? deps.edgeMetrics?.get(edgeMetricKey) : undefined,
    } as Record<string, unknown>,
    ...(DIRECTED_RELATIONS.includes(edge.relation) && {
      markerEnd: { type: MarkerType.ArrowClosed, width: 12, height: 12 },
    }),
  }
}
