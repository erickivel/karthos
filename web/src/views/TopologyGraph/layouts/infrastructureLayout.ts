import { type Node } from '@xyflow/react'
import type { GraphNode } from '@/types/graph'
import type { LayoutDeps, LayoutInput, LayoutResult, LayoutStrategy } from './types'
import { NODE_HEIGHT_SMALL, NODE_WIDTH } from './types'
import { runDagreLayout, type DagreLayoutSpec } from './dagreLayout'
import { namespaceColor } from '../namespaceColor'

// Infrastructure mode renders the K8s hosting hierarchy.
//   - Granularity "Nodes" (overview) and "Pods on nodes" (runtime) need a
//     single tier of grouping (pods inside their host node). Dagre handles
//     that correctly today, so reuse it.
//   - Granularity "Containers in pods" (detailed) needs TWO tiers: containers
//     nested inside pod rectangles, pod rectangles nested inside node
//     rectangles. We hand-pack this because dagre's compound graph cannot
//     express "rectangle inside a rectangle inside a rectangle" cleanly.
//
// The nested layout intentionally drops the `runs_on` and `contains` edges
// from the React Flow output — containment is encoded by visual nesting, so
// drawing the relations on top would only add noise.

const DAGRE_SPEC: DagreLayoutSpec = {
  rankdir: 'LR',
  ranksep: 165,
  nodesep: 115,
  ranker: 'tight-tree',
  acyclicer: 'greedy',
  packNodeGroups: true,
}

const CONTAINER_WIDTH = NODE_WIDTH
const CONTAINER_HEIGHT = NODE_HEIGHT_SMALL

const POD_PADDING_X = 28
const POD_PADDING_TOP = 28
const POD_PADDING_BOTTOM = 40
const NODE_PADDING_X = 32
const NODE_PADDING_TOP = 44
const NODE_PADDING_BOTTOM = 32

const CONTAINER_GAP_X = 20
const CONTAINER_GAP_Y = 32
const POD_GAP_X = 28
const POD_GAP_Y = 32
const NODE_GAP_X = 80
const NODE_ROW_GAP_Y = 80

const POD_MAX_COLS = 2
const NODE_MAX_COLS = 4

interface PodLayout {
  pod: GraphNode
  containers: { node: GraphNode; x: number; y: number }[]
  width: number
  height: number
}

interface HostLayout {
  host: GraphNode
  pods: { layout: PodLayout; x: number; y: number }[]
  width: number
  height: number
}

export const infrastructureLayout: LayoutStrategy = {
  layout(input, deps) {
    const hasContainers = input.nodes.some((n) => n.kind === 'container')
    if (hasContainers) {
      return runInfrastructureNestedLayout(input, deps)
    }
    return runDagreLayout(input, deps, DAGRE_SPEC)
  },
}

function gridDims<T>(items: T[], maxCols: number) {
  if (items.length === 0) return { cols: 0, rows: 0 }
  const cols = Math.min(maxCols, items.length)
  const rows = Math.ceil(items.length / cols)
  return { cols, rows }
}

function layoutPod(pod: GraphNode, containers: GraphNode[]): PodLayout {
  const sorted = containers.slice().sort((a, b) => a.name.localeCompare(b.name))
  const { cols, rows } = gridDims(sorted, POD_MAX_COLS)
  if (cols === 0 || rows === 0) {
    return {
      pod,
      containers: [],
      width: CONTAINER_WIDTH + POD_PADDING_X * 2,
      height: POD_PADDING_TOP + CONTAINER_HEIGHT + POD_PADDING_BOTTOM,
    }
  }
  const width = cols * CONTAINER_WIDTH + (cols - 1) * CONTAINER_GAP_X + POD_PADDING_X * 2
  const positioned = sorted.map((node, index) => {
    const col = index % cols
    const row = Math.floor(index / cols)
    return {
      node,
      // Containers align with the Pod's left inset. This makes a partial last
      // row scan like the rows above it instead of introducing a false centre.
      x: POD_PADDING_X + col * (CONTAINER_WIDTH + CONTAINER_GAP_X),
      y: POD_PADDING_TOP + row * (CONTAINER_HEIGHT + CONTAINER_GAP_Y),
    }
  })
  return {
    pod,
    containers: positioned,
    width,
    height:
      POD_PADDING_TOP +
      rows * CONTAINER_HEIGHT +
      (rows - 1) * CONTAINER_GAP_Y +
      POD_PADDING_BOTTOM,
  }
}

function layoutHost(host: GraphNode, podLayouts: PodLayout[]): HostLayout {
  const { cols, rows } = gridDims(podLayouts, NODE_MAX_COLS)
  if (cols === 0 || rows === 0) {
    return {
      host,
      pods: [],
      width: CONTAINER_WIDTH + NODE_PADDING_X * 2,
      height: NODE_PADDING_TOP + CONTAINER_HEIGHT + NODE_PADDING_BOTTOM,
    }
  }
  const colWidths = Array.from({ length: cols }, () => 0)
  const rowHeights = Array.from({ length: rows }, () => 0)
  podLayouts.forEach((pl, index) => {
    const col = index % cols
    const row = Math.floor(index / cols)
    colWidths[col] = Math.max(colWidths[col] ?? 0, pl.width)
    rowHeights[row] = Math.max(rowHeights[row] ?? 0, pl.height)
  })
  const xOffsets = Array.from({ length: cols }, () => 0)
  {
    let cursor = NODE_PADDING_X
    for (let c = 0; c < cols; c++) {
      xOffsets[c] = cursor
      cursor += (colWidths[c] ?? 0) + POD_GAP_X
    }
  }
  const yOffsets = Array.from({ length: rows }, () => 0)
  {
    let cursor = NODE_PADDING_TOP
    for (let r = 0; r < rows; r++) {
      yOffsets[r] = cursor
      cursor += (rowHeights[r] ?? 0) + POD_GAP_Y
    }
  }
  const positioned = podLayouts.map((layout, index) => {
    const col = index % cols
    const row = Math.floor(index / cols)
    return { layout, x: xOffsets[col] ?? 0, y: yOffsets[row] ?? 0 }
  })
  const totalContentW =
    colWidths.reduce((a, b) => a + b, 0) + (cols - 1) * POD_GAP_X
  const totalContentH =
    rowHeights.reduce((a, b) => a + b, 0) + (rows - 1) * POD_GAP_Y
  return {
    host,
    pods: positioned,
    width: NODE_PADDING_X * 2 + totalContentW,
    height: NODE_PADDING_TOP + totalContentH + NODE_PADDING_BOTTOM,
  }
}

function runInfrastructureNestedLayout(
  input: LayoutInput,
  deps: LayoutDeps,
): LayoutResult {
  const { nodes, groupOf } = input
  if (nodes.length === 0) return { nodes: [], edges: [] }

  const byId = new Map(nodes.map((n) => [n.id, n]))
  const hostNodes = nodes.filter((n) => n.kind === 'node')
  const podsByHost = new Map<string, GraphNode[]>()
  const containersByPod = new Map<string, GraphNode[]>()
  const orphanPods: GraphNode[] = []
  const orphanContainers: GraphNode[] = []

  for (const n of nodes) {
    if (n.kind === 'pod') {
      const host = groupOf.get(n.id)
      if (host && byId.get(host)?.kind === 'node') {
        const list = podsByHost.get(host) ?? []
        list.push(n)
        podsByHost.set(host, list)
      } else {
        orphanPods.push(n)
      }
    } else if (n.kind === 'container') {
      const podId = groupOf.get(n.id)
      if (podId && byId.get(podId)?.kind === 'pod') {
        const list = containersByPod.get(podId) ?? []
        list.push(n)
        containersByPod.set(podId, list)
      } else {
        orphanContainers.push(n)
      }
    }
  }

  hostNodes.sort((a, b) => a.name.localeCompare(b.name))
  const hostLayouts: HostLayout[] = hostNodes.map((host) => {
    const pods = (podsByHost.get(host.id) ?? []).slice().sort((a, b) =>
      a.name.localeCompare(b.name),
    )
    const podLayouts = pods.map((pod) =>
      layoutPod(pod, containersByPod.get(pod.id) ?? []),
    )
    return layoutHost(host, podLayouts)
  })

  // Row-pack hosts left to right; wrap when total width would exceed a soft cap.
  const reactFlowNodes: Node[] = []
  const SOFT_ROW_WIDTH = 2400
  let rowCursorX = 0
  let rowCursorY = 0
  let rowMaxHeight = 0
  for (const hl of hostLayouts) {
    if (rowCursorX > 0 && rowCursorX + hl.width > SOFT_ROW_WIDTH) {
      rowCursorX = 0
      rowCursorY += rowMaxHeight + NODE_ROW_GAP_Y
      rowMaxHeight = 0
    }
    const hostX = rowCursorX
    const hostY = rowCursorY
    rowCursorX += hl.width + NODE_GAP_X
    rowMaxHeight = Math.max(rowMaxHeight, hl.height)

    reactFlowNodes.push({
      id: hl.host.id,
      type: 'namespace',
      position: { x: hostX, y: hostY },
      width: hl.width,
      height: hl.height,
      zIndex: -2,
      selectable: true,
      draggable: false,
      data: {
        ...hl.host,
        variant: 'node',
        nsWidth: hl.width,
        nsHeight: hl.height,
        mode: deps.mode,
      } as Record<string, unknown>,
    })

    for (const { layout: pod, x: px, y: py } of hl.pods) {
      const podX = hostX + px
      const podY = hostY + py
      reactFlowNodes.push({
        id: pod.pod.id,
        type: 'namespace',
        position: { x: podX, y: podY },
        width: pod.width,
        height: pod.height,
        zIndex: -1,
        selectable: true,
        draggable: false,
        data: {
          ...pod.pod,
          variant: 'pod',
          nsWidth: pod.width,
          nsHeight: pod.height,
          accentColor: namespaceColor(pod.pod.namespace),
          mode: deps.mode,
        } as Record<string, unknown>,
      })
      for (const { node, x: cx, y: cy } of pod.containers) {
        reactFlowNodes.push({
          id: node.id,
          type: 'entity',
          position: { x: podX + cx, y: podY + cy },
          width: CONTAINER_WIDTH,
          height: CONTAINER_HEIGHT,
          data: { ...node, mode: deps.mode } as Record<string, unknown>,
        })
      }
    }
  }

  // Orphans (pods without a host visible, containers without a pod visible)
  // get parked in a row under the main layout so they stay reachable instead
  // of silently disappearing.
  if (orphanPods.length > 0 || orphanContainers.length > 0) {
    const orphanY = rowCursorY + rowMaxHeight + NODE_ROW_GAP_Y
    let orphanX = 0
    for (const orphan of [...orphanPods, ...orphanContainers]) {
      reactFlowNodes.push({
        id: orphan.id,
        type: 'entity',
        position: { x: orphanX, y: orphanY },
        width: NODE_WIDTH,
        height: CONTAINER_HEIGHT,
        data: { ...orphan, mode: deps.mode } as Record<string, unknown>,
      })
      orphanX += NODE_WIDTH + 30
    }
  }

  return { nodes: reactFlowNodes, edges: [] }
}
