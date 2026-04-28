import ELK, { type ElkNode, type ElkExtendedEdge } from 'elkjs/lib/elk.bundled.js'
import { MarkerType, Position, type Node, type Edge } from '@xyflow/react'
import type { RelationType } from '@/types/graph'
import type { LayoutStrategy, LayoutInput, LayoutDeps, LayoutResult } from './types'
import { NODE_WIDTH, NODE_HEIGHT_DEFAULT, NODE_HEIGHT_SMALL, SMALL_KINDS } from './types'
import { getEdgeMetricKeyFromNodes } from '../edgeMetrics'

const elk = new ELK()

const NS_PADDING = 30
const NS_PILL_OFFSET = 24
const DIRECTED_RELATIONS: RelationType[] = ['calls', 'runs_on', 'routes_to', 'exposes']

const ELK_OPTIONS: Record<string, string> = {
  'elk.algorithm': 'layered',
  'elk.direction': 'DOWN',
  'elk.layered.spacing.nodeNodeBetweenLayers': '60',
  'elk.spacing.nodeNode': '40',
  'elk.layered.nodePlacement.strategy': 'NETWORK_SIMPLEX',
  'elk.layered.crossingMinimization.strategy': 'LAYER_SWEEP',
  'elk.hierarchyHandling': 'INCLUDE_CHILDREN',
}

export const ownershipLayout: LayoutStrategy = {
  async layout(input: LayoutInput, deps: LayoutDeps): Promise<LayoutResult> {
    return runLayered(input, deps, ELK_OPTIONS, 'DOWN')
  },
}

export async function runLayered(
  input: LayoutInput,
  deps: LayoutDeps,
  options: Record<string, string>,
  direction: 'DOWN' | 'RIGHT',
  parentOptions?: Record<string, string>,
): Promise<LayoutResult> {
  const { nodes: graphNodes, edges: graphEdges, groupOf, parentKind } = input
  if (graphNodes.length === 0) return { nodes: [], edges: [] }

  const parentIdSet = new Set(groupOf.values())
  const parentNodes = graphNodes.filter(
    (n) => n.kind === parentKind && parentIdSet.has(n.id),
  )
  const parentIds = new Set(parentNodes.map((n) => n.id))
  const contentNodes = graphNodes.filter((n) => !parentIds.has(n.id))
  const contentNodeMap = new Map(contentNodes.map((n) => [n.id, n]))
  const contentEdges = graphEdges.filter(
    (e) => !parentIds.has(e.source) && !parentIds.has(e.target),
  )

  const childrenByParent = new Map<string, ElkNode[]>()
  for (const n of contentNodes) {
    const height = SMALL_KINDS.includes(n.kind) ? NODE_HEIGHT_SMALL : NODE_HEIGHT_DEFAULT
    const elkChild: ElkNode = { id: n.id, width: NODE_WIDTH, height }
    const parentId = groupOf.get(n.id)
    if (parentId && parentIds.has(parentId)) {
      const list = childrenByParent.get(parentId) ?? []
      list.push(elkChild)
      childrenByParent.set(parentId, list)
    } else {
      const list = childrenByParent.get('__root__') ?? []
      list.push(elkChild)
      childrenByParent.set('__root__', list)
    }
  }

  const elkParents: ElkNode[] = parentNodes.map((p) => ({
    id: `group:${p.id}`,
    children: childrenByParent.get(p.id) ?? [],
    layoutOptions: {
      'elk.padding': `[top=${NS_PADDING + NS_PILL_OFFSET},left=${NS_PADDING},bottom=${NS_PADDING},right=${NS_PADDING}]`,
      ...(parentOptions ?? {}),
    },
  }))

  const root: ElkNode = {
    id: 'root',
    layoutOptions: options,
    children: [...elkParents, ...(childrenByParent.get('__root__') ?? [])],
    edges: contentEdges
      .filter((e) => contentNodeMap.has(e.source) && contentNodeMap.has(e.target))
      .map<ElkExtendedEdge>((e) => ({ id: e.id, sources: [e.source], targets: [e.target] })),
  }

  const result = await elk.layout(root)

  const sourcePos = direction === 'RIGHT' ? Position.Right : Position.Bottom
  const targetPos = direction === 'RIGHT' ? Position.Left : Position.Top

  const elkNodeById = new Map<string, ElkNode>()

  function indexElkNodes(parent: ElkNode, offsetX: number, offsetY: number) {
    if (parent.id !== 'root') {
      elkNodeById.set(parent.id, {
        ...parent,
        x: (parent.x ?? 0) + offsetX,
        y: (parent.y ?? 0) + offsetY,
      })
    }
    const baseX = parent.id === 'root' ? 0 : (parent.x ?? 0) + offsetX
    const baseY = parent.id === 'root' ? 0 : (parent.y ?? 0) + offsetY
    for (const c of parent.children ?? []) {
      indexElkNodes(c, baseX, baseY)
    }
  }
  indexElkNodes(result, 0, 0)

  const positioned: Node[] = []
  for (const n of contentNodes) {
    const elkN = elkNodeById.get(n.id)
    if (!elkN) continue
    positioned.push({
      id: n.id,
      type: 'entity',
      position: { x: elkN.x ?? 0, y: elkN.y ?? 0 },
      sourcePosition: sourcePos,
      targetPosition: targetPos,
      data: {
        ...n,
        metrics: deps.nodeMetrics?.get(n.id) ?? {},
        mode: deps.mode,
      } as Record<string, unknown>,
    })
  }

  const parentContainers: Node[] = parentNodes
    .map<Node | null>((p) => {
      const elkP = elkNodeById.get(`group:${p.id}`)
      if (!elkP) return null
      return {
        id: p.id,
        type: 'namespace',
        position: { x: elkP.x ?? 0, y: elkP.y ?? 0 },
        zIndex: -1,
        data: {
          ...p,
          variant: parentKind ?? 'namespace',
          nsWidth: elkP.width ?? 0,
          nsHeight: elkP.height ?? 0,
        } as Record<string, unknown>,
        selectable: true,
        draggable: false,
      }
    })
    .filter((x): x is Node => x !== null)

  const edges: Edge[] = contentEdges.map((e) => {
    const directed = DIRECTED_RELATIONS.includes(e.relation)
    const sourceNode = contentNodeMap.get(e.source)
    const targetNode = contentNodeMap.get(e.target)
    const edgeMetricKey = getEdgeMetricKeyFromNodes(e.relation, sourceNode?.name, targetNode?.name)
    return {
      id: e.id,
      source: e.source,
      target: e.target,
      type: 'relation',
      zIndex: e.relation === 'calls' ? 12 : 6,
      data: {
        ...e,
        sourceName: sourceNode?.name,
        targetName: targetNode?.name,
        metrics: edgeMetricKey ? deps.edgeMetrics?.get(edgeMetricKey) : undefined,
      } as Record<string, unknown>,
      ...(directed && {
        markerEnd: { type: MarkerType.ArrowClosed, width: 12, height: 12 },
      }),
    }
  })

  return { nodes: [...parentContainers, ...positioned], edges }
}
