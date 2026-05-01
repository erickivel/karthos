import dagre from '@dagrejs/dagre'
import { MarkerType, Position, type Edge, type Node } from '@xyflow/react'
import type { GraphEdge, GraphNode, RelationType } from '@/types/graph'
import type { LayoutDeps, LayoutInput, LayoutResult } from './types'
import { NODE_HEIGHT_DEFAULT, NODE_HEIGHT_SMALL, NODE_WIDTH, SMALL_KINDS } from './types'
import { getEdgeMetricKeyFromNodes } from '../edgeMetrics'

export interface DagreLayoutSpec {
  rankdir: 'TB' | 'LR'
  ranksep: number
  nodesep: number
  ranker: 'network-simplex' | 'tight-tree' | 'longest-path'
  acyclicer?: 'greedy'
  packNodeGroups?: boolean
}

const DIRECTED_RELATIONS: RelationType[] = ['calls', 'runs_on', 'routes_to', 'exposes']
const NS_PADDING = 30
const NS_PILL_OFFSET = 24
const GROUP_GRID_GAP_X = 32
const GROUP_GRID_GAP_Y = 34
const GROUP_GRID_MAX_COLUMNS = 4
const GROUP_GAP_X = 170

export function runDagreLayout(
  input: LayoutInput,
  deps: LayoutDeps,
  spec: DagreLayoutSpec,
): LayoutResult {
  const { nodes: graphNodes, edges: graphEdges, groupOf, parentKind } = input
  if (graphNodes.length === 0) return { nodes: [], edges: [] }

  const parentIdSet = new Set(groupOf.values())
  const parentNodes = graphNodes.filter(
    (node) => node.kind === parentKind && parentIdSet.has(node.id),
  )
  const parentIds = new Set(parentNodes.map((node) => node.id))
  const contentNodes = graphNodes.filter((node) => !parentIds.has(node.id))
  const contentNodeMap = new Map(contentNodes.map((node) => [node.id, node]))
  const contentEdges = graphEdges.filter(
    (edge) => !parentIds.has(edge.source) && !parentIds.has(edge.target),
  )

  const graph = new dagre.graphlib.Graph({ compound: true })
  graph.setDefaultEdgeLabel(() => ({}))
  graph.setGraph({
    rankdir: spec.rankdir,
    ranksep: spec.ranksep,
    nodesep: spec.nodesep,
    ranker: spec.ranker,
    ...(spec.acyclicer ? { acyclicer: spec.acyclicer } : {}),
  })

  for (const parent of parentNodes) {
    graph.setNode(`group:${parent.id}`, {})
  }

  for (const node of contentNodes) {
    graph.setNode(node.id, { width: NODE_WIDTH, height: nodeHeight(node) })
    const parentId = groupOf.get(node.id)
    if (parentId && parentIds.has(parentId)) {
      graph.setParent(node.id, `group:${parentId}`)
    }
  }

  for (const edge of contentEdges) {
    if (graph.hasNode(edge.source) && graph.hasNode(edge.target)) {
      graph.setEdge(edge.source, edge.target)
    }
  }

  dagre.layout(graph)

  const sourcePosition = spec.rankdir === 'LR' ? Position.Right : Position.Bottom
  const targetPosition = spec.rankdir === 'LR' ? Position.Left : Position.Top
  let positionedContent = contentNodes.map((node) =>
    toReactNode(node, graph, deps, sourcePosition, targetPosition),
  )

  if (spec.packNodeGroups) {
    positionedContent = packNodeGroupChildren(positionedContent, groupOf, parentIds, graph)
  }

  const parentContainers = buildParentContainers(
    parentNodes,
    parentIds,
    parentKind,
    groupOf,
    positionedContent,
    graph,
  )

  const edges = contentEdges
    .filter((edge) => graph.hasNode(edge.source) && graph.hasNode(edge.target))
    .map((edge) => toReactEdge(edge, contentNodeMap, deps))

  return { nodes: [...parentContainers, ...positionedContent], edges }
}

function toReactNode(
  node: GraphNode,
  graph: dagre.graphlib.Graph,
  deps: LayoutDeps,
  sourcePosition: Position,
  targetPosition: Position,
): Node {
  const position = graph.node(node.id)
  const height = position?.height ?? nodeHeight(node)
  return {
    id: node.id,
    type: 'entity',
    position: {
      x: (position?.x ?? 0) - NODE_WIDTH / 2,
      y: (position?.y ?? 0) - height / 2,
    },
    sourcePosition,
    targetPosition,
    data: {
      ...node,
      metrics: deps.nodeMetrics?.get(node.id) ?? {},
      mode: deps.mode,
    } as Record<string, unknown>,
  }
}

function buildParentContainers(
  parentNodes: GraphNode[],
  parentIds: Set<string>,
  parentKind: LayoutInput['parentKind'],
  groupOf: Map<string, string>,
  positionedContent: Node[],
  graph: dagre.graphlib.Graph,
): Node[] {
  const parentBounds = new Map<
    string,
    { minX: number; minY: number; maxX: number; maxY: number }
  >()

  for (const node of positionedContent) {
    const parentId = groupOf.get(node.id)
    if (!parentId || !parentIds.has(parentId)) continue
    const dagreNode = graph.node(node.id)
    const height = dagreNode?.height ?? NODE_HEIGHT_DEFAULT
    const x = node.position.x
    const y = node.position.y
    const bounds = parentBounds.get(parentId)
    if (bounds) {
      bounds.minX = Math.min(bounds.minX, x)
      bounds.minY = Math.min(bounds.minY, y)
      bounds.maxX = Math.max(bounds.maxX, x + NODE_WIDTH)
      bounds.maxY = Math.max(bounds.maxY, y + height)
    } else {
      parentBounds.set(parentId, {
        minX: x,
        minY: y,
        maxX: x + NODE_WIDTH,
        maxY: y + height,
      })
    }
  }

  return parentNodes
    .filter((parent) => parentBounds.has(parent.id))
    .map((parent) => {
      const bounds = parentBounds.get(parent.id)!
      return {
        id: parent.id,
        type: 'namespace',
        position: {
          x: bounds.minX - NS_PADDING,
          y: bounds.minY - NS_PADDING - NS_PILL_OFFSET,
        },
        zIndex: -1,
        data: {
          ...parent,
          variant: parentKind ?? 'namespace',
          nsWidth: bounds.maxX - bounds.minX + NS_PADDING * 2,
          nsHeight: bounds.maxY - bounds.minY + NS_PADDING * 2 + NS_PILL_OFFSET,
        } as Record<string, unknown>,
        selectable: true,
        draggable: false,
      }
    })
}

function toReactEdge(
  edge: GraphEdge,
  contentNodeMap: Map<string, GraphNode>,
  deps: LayoutDeps,
): Edge {
  const sourceNode = contentNodeMap.get(edge.source)
  const targetNode = contentNodeMap.get(edge.target)
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
      markerEnd: {
        type: MarkerType.ArrowClosed,
        width: 12,
        height: 12,
      },
    }),
  }
}

function packNodeGroupChildren(
  positionedContent: Node[],
  groupOf: Map<string, string>,
  parentIds: Set<string>,
  graph: dagre.graphlib.Graph,
): Node[] {
  const childrenByParent = new Map<string, Node[]>()
  for (const node of positionedContent) {
    const parentId = groupOf.get(node.id)
    if (!parentId || !parentIds.has(parentId)) continue
    const siblings = childrenByParent.get(parentId) ?? []
    siblings.push(node)
    childrenByParent.set(parentId, siblings)
  }

  if (childrenByParent.size === 0) return positionedContent

  const nextPositions = new Map<string, { x: number; y: number }>()
  const groups = [...childrenByParent.entries()]
    .map(([parentId, siblings]) => ({
      parentId,
      siblings,
      minX: Math.min(...siblings.map((node) => node.position.x)),
      minY: Math.min(...siblings.map((node) => node.position.y)),
    }))
    .sort((a, b) => a.minX - b.minX || a.minY - b.minY || a.parentId.localeCompare(b.parentId))

  let cursorX = Math.min(...groups.map((group) => group.minX))
  const originY = Math.min(...groups.map((group) => group.minY))

  for (const group of groups) {
    const siblings = group.siblings
    siblings.sort((a, b) => String(a.data.name ?? a.id).localeCompare(String(b.data.name ?? b.id)))
    const columns = Math.min(GROUP_GRID_MAX_COLUMNS, Math.ceil(Math.sqrt(siblings.length)))
    const groupWidth = columns * NODE_WIDTH + Math.max(0, columns - 1) * GROUP_GRID_GAP_X

    siblings.forEach((node, index) => {
      const dagreNode = graph.node(node.id)
      const height = dagreNode?.height ?? NODE_HEIGHT_DEFAULT
      const col = index % columns
      const row = Math.floor(index / columns)
      nextPositions.set(node.id, {
        x: cursorX + col * (NODE_WIDTH + GROUP_GRID_GAP_X),
        y: originY + row * (height + GROUP_GRID_GAP_Y),
      })
    })
    cursorX += groupWidth + GROUP_GAP_X
  }

  return positionedContent.map((node) => {
    const position = nextPositions.get(node.id)
    return position ? { ...node, position } : node
  })
}

function nodeHeight(node: GraphNode): number {
  return SMALL_KINDS.includes(node.kind) ? NODE_HEIGHT_SMALL : NODE_HEIGHT_DEFAULT
}
