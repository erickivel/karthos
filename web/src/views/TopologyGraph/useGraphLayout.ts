import { useMemo } from 'react'
import dagre from '@dagrejs/dagre'
import { MarkerType, Position, type Node, type Edge } from '@xyflow/react'
import type { GraphNode, GraphEdge, EntityKind, RelationType } from '@/types/graph'
import type { NodeMetricValues } from '@/hooks/useNodeMetrics'
import type { EdgeMetricSnapshot, GraphEdgeMetricData } from './edgeMetrics'
import { getEdgeMetricKeyFromNodes } from './edgeMetrics'
import type { ProjectionMode } from './projection'

interface LayoutSpec {
  rankdir: 'TB' | 'LR'
  ranksep: number
  nodesep: number
  ranker: 'network-simplex' | 'tight-tree' | 'longest-path'
  acyclicer: 'greedy' | undefined
}

// Per-mode layout config. Adding/changing layout per layer is one entry.
const LAYOUT_SPECS: Record<ProjectionMode, LayoutSpec> = {
  ownership: { rankdir: 'TB', ranksep: 100, nodesep: 40, ranker: 'tight-tree', acyclicer: 'greedy' },
  infrastructure: { rankdir: 'TB', ranksep: 100, nodesep: 40, ranker: 'tight-tree', acyclicer: 'greedy' },
  traffic: { rankdir: 'LR', ranksep: 170, nodesep: 86, ranker: 'tight-tree', acyclicer: 'greedy' },
}

const DIRECTED_RELATIONS: RelationType[] = ['calls', 'runs_on', 'routes_to', 'exposes']

const NODE_WIDTH = 240
const NODE_HEIGHT_DEFAULT = 170
const NODE_HEIGHT_SMALL = 90
const NS_PADDING = 30
const NS_PILL_OFFSET = 24
const GROUP_GRID_GAP_X = 28
const GROUP_GRID_GAP_Y = 30
const GROUP_GRID_MAX_COLUMNS = 4
const GROUP_GAP_X = 160

const SMALL_KINDS: EntityKind[] = ['configmap', 'secret', 'pvc']

interface LayoutResult {
  nodes: Node[]
  edges: Edge[]
}

export interface LayoutInput {
  nodes: GraphNode[]
  edges: GraphEdge[]
  groupOf: Map<string, string>
  parentKind: EntityKind | null
}

export function useGraphLayout(
  input: LayoutInput,
  mode: ProjectionMode,
  nodeMetrics?: Map<string, NodeMetricValues>,
  edgeMetrics?: Map<string, EdgeMetricSnapshot>,
): LayoutResult {
  const { nodes: graphNodes, edges: graphEdges, groupOf, parentKind } = input
  const layoutSpec = LAYOUT_SPECS[mode]

  return useMemo(() => {
    if (graphNodes.length === 0) return { nodes: [], edges: [] }

    // Parent containers are entities whose kind matches parentKind AND that are
    // referenced as a parent in groupOf. Unreferenced parent-kind nodes render
    // as normal cards, which keeps infrastructure overview useful when only
    // Kubernetes node entities are visible.
    const parentIdSet = new Set(groupOf.values())
    const parentNodes = graphNodes.filter(
      (n) => n.kind === parentKind && parentIdSet.has(n.id),
    )
    const parentIds = new Set(parentNodes.map((n) => n.id))
    const contentNodes = graphNodes.filter((n) => !parentIds.has(n.id))
    const contentNodeMap = new Map(contentNodes.map((node) => [node.id, node]))

    // Drop edges that touch a parent-container node (containers don't render
    // as graph-level entities in the layout).
    const contentEdges = graphEdges.filter(
      (e) => !parentIds.has(e.source) && !parentIds.has(e.target),
    )

    const g = new dagre.graphlib.Graph({ compound: true })
    g.setDefaultEdgeLabel(() => ({}))
    g.setGraph({
      rankdir: layoutSpec.rankdir,
      ranksep: layoutSpec.ranksep,
      nodesep: layoutSpec.nodesep,
      ranker: layoutSpec.ranker,
      ...(layoutSpec.acyclicer ? { acyclicer: layoutSpec.acyclicer } : {}),
    })

    for (const parent of parentNodes) {
      g.setNode(`group:${parent.id}`, {})
    }

    for (const node of contentNodes) {
      const height = SMALL_KINDS.includes(node.kind) ? NODE_HEIGHT_SMALL : NODE_HEIGHT_DEFAULT
      g.setNode(node.id, { width: NODE_WIDTH, height })
      const parentId = groupOf.get(node.id)
      if (parentId && parentIds.has(parentId)) {
        g.setParent(node.id, `group:${parentId}`)
      }
    }

    for (const edge of contentEdges) {
      if (g.hasNode(edge.source) && g.hasNode(edge.target)) {
        g.setEdge(edge.source, edge.target)
      }
    }

    dagre.layout(g)

    const sourcePos = layoutSpec.rankdir === 'LR' ? Position.Right : Position.Bottom
    const targetPos = layoutSpec.rankdir === 'LR' ? Position.Left : Position.Top

    let positionedContent: Node[] = contentNodes.map((node) => {
      const pos = g.node(node.id)
      const h = pos?.height ?? NODE_HEIGHT_DEFAULT
      const metrics = nodeMetrics?.get(node.id) ?? {}
      return {
        id: node.id,
        type: 'entity' as const,
        position: { x: (pos?.x ?? 0) - NODE_WIDTH / 2, y: (pos?.y ?? 0) - h / 2 },
        sourcePosition: sourcePos,
        targetPosition: targetPos,
        data: { ...node, metrics } as Record<string, unknown>,
      }
    })

    if (parentKind === 'node') {
      positionedContent = packNodeGroupChildren(positionedContent, groupOf, parentIds, g)
    }

    const parentBounds = new Map<
      string,
      { minX: number; minY: number; maxX: number; maxY: number }
    >()
    for (const rfNode of positionedContent) {
      const parentId = groupOf.get(rfNode.id)
      if (!parentId || !parentIds.has(parentId)) continue
      const dagreNode = g.node(rfNode.id)
      const w = NODE_WIDTH
      const h = dagreNode?.height ?? NODE_HEIGHT_DEFAULT
      const x = rfNode.position.x
      const y = rfNode.position.y
      const bounds = parentBounds.get(parentId)
      if (bounds) {
        bounds.minX = Math.min(bounds.minX, x)
        bounds.minY = Math.min(bounds.minY, y)
        bounds.maxX = Math.max(bounds.maxX, x + w)
        bounds.maxY = Math.max(bounds.maxY, y + h)
      } else {
        parentBounds.set(parentId, { minX: x, minY: y, maxX: x + w, maxY: y + h })
      }
    }

    const parentContainerNodes: Node[] = parentNodes
      .filter((p) => parentBounds.has(p.id))
      .map((p) => {
        const b = parentBounds.get(p.id)!
        return {
          id: p.id,
          type: 'namespace' as const,
          position: {
            x: b.minX - NS_PADDING,
            y: b.minY - NS_PADDING - NS_PILL_OFFSET,
          },
          zIndex: -1,
          data: {
            ...p,
            variant: parentKind ?? 'namespace',
            nsWidth: b.maxX - b.minX + NS_PADDING * 2,
            nsHeight: b.maxY - b.minY + NS_PADDING * 2 + NS_PILL_OFFSET,
          } as Record<string, unknown>,
          selectable: true,
          draggable: false,
        }
      })

    const nodes = [...parentContainerNodes, ...positionedContent]

    const edges: Edge[] = contentEdges
      .filter((edge) => g.hasNode(edge.source) && g.hasNode(edge.target))
      .map((edge) => {
        const directed = DIRECTED_RELATIONS.includes(edge.relation)
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
          type: 'relation' as const,
          zIndex: edge.relation === 'calls' ? 12 : 6,
          data: {
            ...edge,
            sourceName: sourceNode?.name,
            targetName: targetNode?.name,
            metrics: edgeMetricKey ? edgeMetrics?.get(edgeMetricKey) : undefined,
          } as GraphEdge & GraphEdgeMetricData & Record<string, unknown>,
          ...(directed && {
            markerEnd: {
              type: MarkerType.ArrowClosed,
              width: 12,
              height: 12,
            },
          }),
        }
      })

    return { nodes, edges }
  }, [graphNodes, graphEdges, groupOf, parentKind, nodeMetrics, edgeMetrics, layoutSpec])
}

function packNodeGroupChildren(
  positionedContent: Node[],
  groupOf: Map<string, string>,
  parentIds: Set<string>,
  g: dagre.graphlib.Graph,
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
      const dagreNode = g.node(node.id)
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
