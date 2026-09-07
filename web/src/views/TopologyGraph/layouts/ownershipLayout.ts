import { MarkerType, Position, type Node, type Edge } from '@xyflow/react'
import type { EntityKind, GraphEdge, GraphNode, RelationType } from '@/types/graph'
import type { LayoutStrategy, LayoutInput, LayoutDeps, LayoutResult } from './types'
import { NODE_WIDTH, NODE_HEIGHT_DEFAULT } from './types'

const DIRECTED_RELATIONS: RelationType[] = ['calls', 'runs_on', 'routes_to', 'exposes']
const DEPENDENCY_KINDS = new Set<EntityKind>(['configmap', 'secret', 'pvc'])
const ENDPOINT_KINDS = new Set<EntityKind>(['ingress', 'service'])

const OWNERSHIP_TREE = {
  namespaceGap: 170,
  namespacePaddingX: 48,
  namespacePaddingTop: 82,
  namespacePaddingBottom: 48,
  emptyNamespaceWidth: 360,
  emptyNamespaceHeight: 180,
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
  edgeRoutes: Map<string, OwnershipEdgeRoute>
}

interface OwnershipEdgeRoute {
  sourceHandle: string
  targetHandle: string
  routePoints: Array<{ x: number; y: number }>
}

interface RankedEdge {
  edge: GraphEdge
  source: string
  target: string
}

interface ComponentModel {
  nodes: GraphNode[]
  edges: RankedEdge[]
  hierarchyEdges: RankedEdge[]
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
    (n) => n.kind === parentKind && (parentKind === 'namespace' || parentIdSet.has(n.id)),
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
  const edgeRoutes = new Map<string, OwnershipEdgeRoute>()
  // Ownership stacks namespaces top-to-bottom (one rectangle per row) so each
  // namespace's tree gets the full canvas width and reads vertically. Earlier
  // versions packed them side-by-side, which made wide namespaces overlap and
  // forced horizontal scrolling — see frontend-design-spec § 5.2.
  let nextNamespaceY = 0

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
    const namespaceNodeIds = new Set(namespaceNodes.map((node) => node.id))
    const namespaceEdges = contentEdges.filter(
      (edge) => namespaceNodeIds.has(edge.source) && namespaceNodeIds.has(edge.target),
    )
    const components = splitComponents(namespaceNodes, namespaceEdges)
      .map((componentNodes) => buildComponentModel(componentNodes, namespaceEdges))
      .map(layoutOwnershipComponent)

    const hasContent = components.length > 0
    const contentWidth = hasContent
      ? components.reduce(
          (sum, component, index) =>
            sum + component.width + (index > 0 ? OWNERSHIP_TREE.componentGap : 0),
          0,
        )
      : 0
    const contentHeight = hasContent
      ? Math.max(...components.map((component) => component.height))
      : 0
    const namespaceWidth = hasContent
      ? contentWidth + OWNERSHIP_TREE.namespacePaddingX * 2
      : OWNERSHIP_TREE.emptyNamespaceWidth
    const namespaceHeight = hasContent
      ? contentHeight +
        OWNERSHIP_TREE.namespacePaddingTop +
        OWNERSHIP_TREE.namespacePaddingBottom
      : OWNERSHIP_TREE.emptyNamespaceHeight
    const namespaceX = 0
    const namespaceY = nextNamespaceY
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
          width: namespaceWidth,
          height: namespaceHeight,
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
      for (const [edgeID, route] of component.edgeRoutes) {
        edgeRoutes.set(edgeID, {
          ...route,
          routePoints: route.routePoints.map((point) => ({
            x: componentX + point.x,
            y: baseY + point.y,
          })),
        })
      }
      componentX += component.width + OWNERSHIP_TREE.componentGap
    }

    nextNamespaceY += namespaceHeight + OWNERSHIP_TREE.namespaceGap
  }

  const edges = contentEdges.map((edge) => toReactEdge(edge, contentNodeMap, edgeRoutes.get(edge.id)))

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
    edges: rankedEdges,
    hierarchyEdges: rankedEdges.filter((edge) => edge.edge.relation === 'contains'),
  }
}

function layoutOwnershipComponent(component: ComponentModel): ComponentLayout {
  const nodeById = new Map(component.nodes.map((n) => [n.id, n]))
  const endpointNodes = sortOwnershipNodes(
    component.nodes.filter((node) => ENDPOINT_KINDS.has(node.kind)),
  )
  const allDependencyNodes = sortOwnershipNodes(
    component.nodes.filter((node) => DEPENDENCY_KINDS.has(node.kind)),
  )
  const dependencyNodes = allDependencyNodes
  const hierarchyNodes = component.nodes.filter(
    (node) => !ENDPOINT_KINDS.has(node.kind) && !DEPENDENCY_KINDS.has(node.kind),
  )
  const hierarchyNodeById = new Map(hierarchyNodes.map((node) => [node.id, node]))
  const incomingByNode = new Map<string, RankedEdge[]>()
  for (const node of hierarchyNodes) incomingByNode.set(node.id, [])
  for (const edge of component.hierarchyEdges) {
    if (!hierarchyNodeById.has(edge.source) || !hierarchyNodeById.has(edge.target)) continue
    incomingByNode.get(edge.target)!.push(edge)
  }

  const parentOf = new Map<string, string | null>()
  for (const node of hierarchyNodes) {
    const incoming = incomingByNode.get(node.id) ?? []
    if (incoming.length === 0) {
      parentOf.set(node.id, null)
      continue
    }
    const parents = incoming
      .map((edge) => hierarchyNodeById.get(edge.source))
      .filter(isGraphNode)
      .sort(compareOwnershipNodes)
    parentOf.set(node.id, parents[0]?.id ?? null)
  }

  const childrenOf = new Map<string, GraphNode[]>()
  for (const node of hierarchyNodes) childrenOf.set(node.id, [])
  for (const node of hierarchyNodes) {
    const parent = parentOf.get(node.id)
    if (parent && parent !== node.id) childrenOf.get(parent)!.push(node)
  }
  for (const list of childrenOf.values()) list.sort(compareOwnershipNodes)

  const roots = sortOwnershipNodes(
    hierarchyNodes.filter((n) => parentOf.get(n.id) == null),
  )

  // `routes_to` is not a containment relation, but it does have an explicit
  // visual direction in Ownership mode: ingress sits above the Service it
  // routes to. Compute only endpoint ranks here; `contains` remains the sole
  // hierarchy parent relationship.
  const endpointByID = new Map(endpointNodes.map((node) => [node.id, node]))
  const endpointRanks = new Map(endpointNodes.map((node) => [node.id, 0]))
  for (let pass = 0; pass < endpointNodes.length; pass += 1) {
    let changed = false
    for (const edge of component.edges) {
      if (edge.edge.relation !== 'routes_to') continue
      if (!endpointByID.has(edge.source) || !endpointByID.has(edge.target)) continue
      const nextRank = (endpointRanks.get(edge.source) ?? 0) + 1
      if (nextRank > (endpointRanks.get(edge.target) ?? 0)) {
        endpointRanks.set(edge.target, nextRank)
        changed = true
      }
    }
    if (!changed) break
  }
  const endpointsByRank = new Map<number, GraphNode[]>()
  for (const endpoint of endpointNodes) {
    const rank = endpointRanks.get(endpoint.id) ?? 0
    const row = endpointsByRank.get(rank) ?? []
    row.push(endpoint)
    endpointsByRank.set(rank, row)
  }
  for (const row of endpointsByRank.values()) row.sort(compareOwnershipNodes)
  const endpointRankCount = endpointNodes.length === 0
    ? 0
    : Math.max(...endpointsByRank.keys()) + 1

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
  for (const node of hierarchyNodes) computeWidth(node.id, new Set())

  const positioned: PositionedNode[] = []
  const visited = new Set<string>()
  const treeRootWidth = rowWidth(roots, (node) => widthOf.get(node.id) ?? NODE_WIDTH)
  const endpointWidth = Math.max(
    0,
    ...[...endpointsByRank.values()].map((row) => rowWidth(row, () => NODE_WIDTH)),
  )
  const dependencyWidth = rowWidth(dependencyNodes, () => NODE_WIDTH)
  const spineWidth = Math.max(NODE_WIDTH, treeRootWidth, endpointWidth, dependencyWidth)
  let maxDepth = endpointRankCount > 0 ? endpointRankCount - 1 : -1

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

  for (const [rank, endpoints] of [...endpointsByRank.entries()].sort(([a], [b]) => a - b)) {
    placeRow(endpoints, rank * OWNERSHIP_TREE.rankGap, spineWidth, positioned)
  }

  let cursor = (spineWidth - treeRootWidth) / 2
  const treeStartDepth = endpointRankCount
  for (const root of roots) {
    place(root.id, cursor, treeStartDepth)
    cursor += (widthOf.get(root.id) ?? NODE_WIDTH) + OWNERSHIP_TREE.siblingGap
  }
  for (const node of hierarchyNodes) {
    if (visited.has(node.id)) continue
    place(node.id, cursor, treeStartDepth)
    cursor += (widthOf.get(node.id) ?? NODE_WIDTH) + OWNERSHIP_TREE.siblingGap
  }

  if (dependencyNodes.length > 0) {
    maxDepth += 1
    placeRow(
      dependencyNodes,
      maxDepth * OWNERSHIP_TREE.rankGap,
      spineWidth,
      positioned,
    )
  }

  const edgeRoutes = new Map<string, OwnershipEdgeRoute>()

  const treeHeight = maxDepth < 0
    ? NODE_HEIGHT_DEFAULT
    : maxDepth * OWNERSHIP_TREE.rankGap + NODE_HEIGHT_DEFAULT
  return {
    nodes: positioned,
    width: spineWidth,
    height: treeHeight,
    edgeRoutes,
  }
}

function rowWidth(
  nodes: GraphNode[],
  widthFor: (node: GraphNode) => number,
): number {
  return nodes.reduce(
    (total, node, index) => total + widthFor(node) + (index > 0 ? OWNERSHIP_TREE.siblingGap : 0),
    0,
  )
}

function placeRow(
  nodes: GraphNode[],
  y: number,
  totalWidth: number,
  positioned: PositionedNode[],
): void {
  const width = rowWidth(nodes, () => NODE_WIDTH)
  let x = (totalWidth - width) / 2
  for (const node of nodes) {
    positioned.push({ node, x, y })
    x += NODE_WIDTH + OWNERSHIP_TREE.siblingGap
  }
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
    // Explicit size so MiniMap can render a placeholder for each node — the
    // custom EntityNode doesn't populate `node.measured` in v12.
    width: NODE_WIDTH,
    height: NODE_HEIGHT_DEFAULT,
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
  route: OwnershipEdgeRoute | undefined,
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
    ...(route && {
      sourceHandle: route.sourceHandle,
      targetHandle: route.targetHandle,
    }),
    data: {
      ...edge,
      sourceName: sourceNode?.name,
      targetName: targetNode?.name,
      routeStyle: 'ownership',
      ...(route && { routePoints: route.routePoints }),
    } as Record<string, unknown>,
    ...(directed && {
      markerEnd: { type: MarkerType.ArrowClosed, width: 12, height: 12 },
    }),
  }
}
