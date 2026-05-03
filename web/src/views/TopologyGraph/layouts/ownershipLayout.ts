import { MarkerType, Position, type Node, type Edge } from '@xyflow/react'
import type { EntityKind, GraphEdge, GraphNode, RelationType } from '@/types/graph'
import type { LayoutStrategy, LayoutInput, LayoutDeps, LayoutResult } from './types'
import { NODE_WIDTH, NODE_HEIGHT_DEFAULT } from './types'

const DIRECTED_RELATIONS: RelationType[] = ['calls', 'runs_on', 'routes_to', 'exposes']

const OWNERSHIP_TREE = {
  namespaceGap: 170,
  namespacePaddingX: 48,
  namespacePaddingTop: 82,
  namespacePaddingBottom: 48,
  componentGap: 180,
  siblingGap: 62,
  rankGap: 250,
}

const KIND_ORDER: Partial<Record<EntityKind, number>> = {
  ingress: 10,
  service: 20,
  workload: 30,
  pod: 40,
  configmap: 50,
  secret: 60,
  pvc: 70,
  container: 80,
}

const RELATION_ORDER: Partial<Record<RelationType, number>> = {
  routes_to: 10,
  exposes: 20,
  contains: 30,
  uses: 40,
  mounts: 50,
}

interface PositionedNode {
  node: GraphNode
  x: number
  y: number
}

interface ComponentLayout {
  nodes: PositionedNode[]
  width: number
  height: number
}

interface RankedEdge {
  edge: GraphEdge
  source: string
  target: string
}

interface ComponentModel {
  nodes: GraphNode[]
  ranks: Map<string, number>
  edges: RankedEdge[]
}

export const ownershipLayout: LayoutStrategy = {
  layout(input: LayoutInput, deps: LayoutDeps): LayoutResult {
    return runOwnershipTree(input, deps)
  },
}

function runOwnershipTree(input: LayoutInput, deps: LayoutDeps): LayoutResult {
  const { nodes: graphNodes, edges: graphEdges, groupOf, parentKind } = input
  if (graphNodes.length === 0) return { nodes: [], edges: [] }

  const parentIdSet = new Set(groupOf.values())
  const parentNodes = graphNodes.filter(
    (n) => n.kind === parentKind && parentIdSet.has(n.id),
  )
  const parentIds = new Set(parentNodes.map((n) => n.id))
  const parentById = new Map(parentNodes.map((n) => [n.id, n]))
  const contentNodes = graphNodes.filter((n) => !parentIds.has(n.id))
  const contentNodeMap = new Map(contentNodes.map((n) => [n.id, n]))
  const contentEdges = graphEdges.filter(
    (e) => contentNodeMap.has(e.source) && contentNodeMap.has(e.target),
  )

  const nodesByParent = new Map<string, GraphNode[]>()
  for (const node of contentNodes) {
    const parentId = groupOf.get(node.id)
    const key = parentId && parentIds.has(parentId) ? parentId : '__root__'
    const list = nodesByParent.get(key) ?? []
    list.push(node)
    nodesByParent.set(key, list)
  }

  const positioned: Node[] = []
  const parentContainers: Node[] = []
  let nextNamespaceX = 0

  const parentOrder = [
    ...parentNodes,
    ...(nodesByParent.has('__root__')
      ? [{
          id: '__root__',
          kind: 'namespace' as const,
          name: '__root__',
          namespace: '',
          cluster: '',
          status: 'unknown',
        }]
      : []),
  ]

  for (const parent of parentOrder) {
    const namespaceNodes = sortOwnershipNodes(nodesByParent.get(parent.id) ?? [])
    if (namespaceNodes.length === 0) continue

    const namespaceNodeIds = new Set(namespaceNodes.map((node) => node.id))
    const namespaceEdges = contentEdges.filter(
      (edge) => namespaceNodeIds.has(edge.source) && namespaceNodeIds.has(edge.target),
    )
    const components = splitComponents(namespaceNodes, namespaceEdges)
      .map((componentNodes) => buildComponentModel(componentNodes, namespaceEdges))
      .map(layoutOwnershipComponent)

    const contentWidth = components.reduce(
      (sum, component, index) =>
        sum + component.width + (index > 0 ? OWNERSHIP_TREE.componentGap : 0),
      0,
    )
    const contentHeight = Math.max(...components.map((component) => component.height))
    const namespaceWidth = contentWidth + OWNERSHIP_TREE.namespacePaddingX * 2
    const namespaceHeight =
      contentHeight +
      OWNERSHIP_TREE.namespacePaddingTop +
      OWNERSHIP_TREE.namespacePaddingBottom
    const namespaceX = nextNamespaceX
    const namespaceY = 0
    const isRootGroup = parent.id === '__root__'
    let componentX = namespaceX + (isRootGroup ? 0 : OWNERSHIP_TREE.namespacePaddingX)
    const baseY = namespaceY + (isRootGroup ? 0 : OWNERSHIP_TREE.namespacePaddingTop)

    if (!isRootGroup) {
      const realParent = parentById.get(parent.id)
      if (realParent) {
        parentContainers.push({
          id: realParent.id,
          type: 'namespace',
          position: { x: namespaceX, y: namespaceY },
          zIndex: -1,
          data: {
            ...realParent,
            variant: parentKind ?? 'namespace',
            nsWidth: namespaceWidth,
            nsHeight: namespaceHeight,
          } as Record<string, unknown>,
          selectable: true,
          draggable: false,
        })
      }
    }

    for (const component of components) {
      for (const item of component.nodes) {
        const x = componentX + item.x
        const y = baseY + item.y
        positioned.push(toReactNode(item.node, x, y, deps))
      }
      componentX += component.width + OWNERSHIP_TREE.componentGap
    }

    nextNamespaceX += namespaceWidth + OWNERSHIP_TREE.namespaceGap
  }

  const edges = contentEdges.map((edge) => toReactEdge(edge, contentNodeMap))

  return { nodes: [...parentContainers, ...positioned], edges }
}

function splitComponents(nodes: GraphNode[], edges: GraphEdge[]): GraphNode[][] {
  const nodeById = new Map(nodes.map((node) => [node.id, node]))
  const adjacency = new Map(nodes.map((node) => [node.id, new Set<string>()]))
  for (const edge of edges) {
    if (!nodeById.has(edge.source) || !nodeById.has(edge.target)) continue
    adjacency.get(edge.source)?.add(edge.target)
    adjacency.get(edge.target)?.add(edge.source)
  }

  const visited = new Set<string>()
  const components: GraphNode[][] = []
  for (const node of nodes) {
    if (visited.has(node.id)) continue
    const stack = [node.id]
    const ids: string[] = []
    visited.add(node.id)
    while (stack.length > 0) {
      const id = stack.pop()
      if (!id) continue
      ids.push(id)
      for (const next of adjacency.get(id) ?? []) {
        if (visited.has(next)) continue
        visited.add(next)
        stack.push(next)
      }
    }
    components.push(sortOwnershipNodes(ids.map((id) => nodeById.get(id)).filter(isGraphNode)))
  }

  return components.sort((a, b) => componentSortKey(a).localeCompare(componentSortKey(b)))
}

function buildComponentModel(nodes: GraphNode[], edges: GraphEdge[]): ComponentModel {
  const nodeById = new Map(nodes.map((node) => [node.id, node]))
  const rankedEdges = sortRankedEdges(
    edges
      .filter((edge) => nodeById.has(edge.source) && nodeById.has(edge.target))
      .map((edge) => ({ edge, source: edge.source, target: edge.target })),
    nodeById,
  )
  return {
    nodes: sortOwnershipNodes(nodes),
    ranks: computeDynamicRanks(nodes, rankedEdges),
    edges: rankedEdges,
  }
}

function layoutOwnershipComponent(component: ComponentModel): ComponentLayout {
  const nodeById = new Map(component.nodes.map((n) => [n.id, n]))
  const incomingByNode = new Map<string, RankedEdge[]>()
  for (const node of component.nodes) incomingByNode.set(node.id, [])
  for (const edge of component.edges) {
    if (!nodeById.has(edge.source) || !nodeById.has(edge.target)) continue
    incomingByNode.get(edge.target)!.push(edge)
  }

  const parentOf = new Map<string, string | null>()
  for (const node of component.nodes) {
    const incoming = incomingByNode.get(node.id) ?? []
    if (incoming.length === 0) {
      parentOf.set(node.id, null)
      continue
    }
    let bestParent: string | null = null
    let bestRank = Infinity
    for (const edge of incoming) {
      if (edge.source === node.id) continue
      const rank = component.ranks.get(edge.source) ?? 0
      if (rank < bestRank) {
        bestRank = rank
        bestParent = edge.source
      } else if (
        rank === bestRank &&
        bestParent !== null &&
        compareOwnershipNodes(nodeById.get(edge.source)!, nodeById.get(bestParent)!) < 0
      ) {
        bestParent = edge.source
      }
    }
    parentOf.set(node.id, bestParent)
  }

  const childrenOf = new Map<string, GraphNode[]>()
  for (const node of component.nodes) childrenOf.set(node.id, [])
  for (const node of component.nodes) {
    const parent = parentOf.get(node.id)
    if (parent && parent !== node.id) childrenOf.get(parent)!.push(node)
  }
  for (const list of childrenOf.values()) list.sort(compareOwnershipNodes)

  const roots = sortOwnershipNodes(
    component.nodes.filter((n) => parentOf.get(n.id) == null),
  )

  const widthOf = new Map<string, number>()
  const computeWidth = (id: string, stack: Set<string>): number => {
    const cached = widthOf.get(id)
    if (cached !== undefined) return cached
    if (stack.has(id)) return NODE_WIDTH
    stack.add(id)
    const children = childrenOf.get(id) ?? []
    let total = 0
    children.forEach((child, i) => {
      total += computeWidth(child.id, stack)
      if (i < children.length - 1) total += OWNERSHIP_TREE.siblingGap
    })
    stack.delete(id)
    const w = Math.max(NODE_WIDTH, total)
    widthOf.set(id, w)
    return w
  }
  for (const root of roots) computeWidth(root.id, new Set())
  for (const node of component.nodes) computeWidth(node.id, new Set())

  const positioned: PositionedNode[] = []
  const visited = new Set<string>()
  let maxDepth = 0

  const place = (id: string, leftEdge: number, depth: number) => {
    if (visited.has(id)) return
    visited.add(id)
    maxDepth = Math.max(maxDepth, depth)
    const w = widthOf.get(id) ?? NODE_WIDTH
    const node = nodeById.get(id)!
    positioned.push({
      node,
      x: leftEdge + (w - NODE_WIDTH) / 2,
      y: depth * OWNERSHIP_TREE.rankGap,
    })
    const children = (childrenOf.get(id) ?? []).filter((c) => !visited.has(c.id))
    if (children.length === 0) return
    let childTotal = 0
    children.forEach((child, i) => {
      childTotal += widthOf.get(child.id) ?? NODE_WIDTH
      if (i < children.length - 1) childTotal += OWNERSHIP_TREE.siblingGap
    })
    let cursor = leftEdge + (w - childTotal) / 2
    for (const child of children) {
      place(child.id, cursor, depth + 1)
      cursor += (widthOf.get(child.id) ?? NODE_WIDTH) + OWNERSHIP_TREE.siblingGap
    }
  }

  let cursor = 0
  for (const root of roots) {
    place(root.id, cursor, 0)
    cursor += (widthOf.get(root.id) ?? NODE_WIDTH) + OWNERSHIP_TREE.siblingGap
  }
  for (const node of component.nodes) {
    if (visited.has(node.id)) continue
    place(node.id, cursor, 0)
    cursor += (widthOf.get(node.id) ?? NODE_WIDTH) + OWNERSHIP_TREE.siblingGap
  }

  const totalWidth = Math.max(NODE_WIDTH, cursor - OWNERSHIP_TREE.siblingGap)
  return {
    nodes: positioned,
    width: totalWidth,
    height: maxDepth * OWNERSHIP_TREE.rankGap + NODE_HEIGHT_DEFAULT,
  }
}

function computeDynamicRanks(nodes: GraphNode[], edges: RankedEdge[]): Map<string, number> {
  const ranks = new Map(nodes.map((node) => [node.id, 0]))
  const incomingCount = new Map(nodes.map((node) => [node.id, 0]))
  const outgoing = new Map(nodes.map((node) => [node.id, [] as RankedEdge[]]))

  for (const edge of edges) {
    incomingCount.set(edge.target, (incomingCount.get(edge.target) ?? 0) + 1)
    const list = outgoing.get(edge.source) ?? []
    list.push(edge)
    outgoing.set(edge.source, list)
  }

  const roots = sortOwnershipNodes(
    nodes.filter((node) => (incomingCount.get(node.id) ?? 0) === 0),
  )
  const fallbackRoot = sortOwnershipNodes(nodes)[0]
  const traversalRoots = roots.length > 0 ? roots : fallbackRoot ? [fallbackRoot] : []
  const visiting = new Set<string>()

  const visit = (nodeId: string, rank: number) => {
    ranks.set(nodeId, Math.max(ranks.get(nodeId) ?? 0, rank))
    if (visiting.has(nodeId)) return

    visiting.add(nodeId)
    for (const edge of outgoing.get(nodeId) ?? []) {
      if (visiting.has(edge.target)) continue
      visit(edge.target, (ranks.get(nodeId) ?? rank) + 1)
    }
    visiting.delete(nodeId)
  }

  for (const root of traversalRoots) {
    visit(root.id, ranks.get(root.id) ?? 0)
  }

  return ranks
}

function sortOwnershipNodes(nodes: GraphNode[]): GraphNode[] {
  return [...nodes].sort(compareOwnershipNodes)
}

function compareOwnershipNodes(a: GraphNode, b: GraphNode): number {
  const kindDelta = kindOrder(a.kind) - kindOrder(b.kind)
  if (kindDelta !== 0) return kindDelta
  return a.name.localeCompare(b.name)
}

function sortRankedEdges(
  edges: RankedEdge[],
  nodeById: Map<string, GraphNode>,
): RankedEdge[] {
  return [...edges].sort((a, b) => {
    const relationDelta = relationOrder(a.edge.relation) - relationOrder(b.edge.relation)
    if (relationDelta !== 0) return relationDelta
    const sourceDelta = compareOwnershipNodes(nodeById.get(a.source)!, nodeById.get(b.source)!)
    if (sourceDelta !== 0) return sourceDelta
    return compareOwnershipNodes(nodeById.get(a.target)!, nodeById.get(b.target)!)
  })
}

function componentSortKey(nodes: GraphNode[]): string {
  const top = sortOwnershipNodes(nodes)[0]
  if (!top) return ''
  return `${kindOrder(top.kind)}:${top.kind}:${top.name}`
}

function kindOrder(kind: EntityKind): number {
  return KIND_ORDER[kind] ?? 999
}

function relationOrder(relation: RelationType): number {
  return RELATION_ORDER[relation] ?? 999
}

function isGraphNode(node: GraphNode | undefined): node is GraphNode {
  return node !== undefined
}

function toReactNode(node: GraphNode, x: number, y: number, deps: LayoutDeps): Node {
  return {
    id: node.id,
    type: 'entity',
    position: { x, y },
    sourcePosition: Position.Bottom,
    targetPosition: Position.Top,
    data: {
      ...node,
      mode: deps.mode,
    } as Record<string, unknown>,
  }
}

function toReactEdge(
  edge: GraphEdge,
  contentNodeMap: Map<string, GraphNode>,
): Edge {
  const directed = DIRECTED_RELATIONS.includes(edge.relation)
  const sourceNode = contentNodeMap.get(edge.source)
  const targetNode = contentNodeMap.get(edge.target)

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
      routeStyle: 'ownership',
    } as Record<string, unknown>,
    ...(directed && {
      markerEnd: { type: MarkerType.ArrowClosed, width: 12, height: 12 },
    }),
  }
}
