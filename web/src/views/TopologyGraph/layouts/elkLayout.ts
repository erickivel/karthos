import ELK from 'elkjs/lib/elk.bundled.js'
import type { ElkExtendedEdge, ElkNode } from 'elkjs/lib/elk-api'
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
  type MeasuredSize,
} from './types'

const elk = new ELK()

const DIRECTED_RELATIONS: RelationType[] = ['calls', 'routes_to', 'exposes']

const ELK_LAYOUT_OPTIONS: Record<string, string> = {
  'elk.algorithm': 'layered',
  'elk.direction': 'RIGHT',
  'elk.edgeRouting': 'ORTHOGONAL',
  'org.eclipse.elk.edgeRouting': 'ORTHOGONAL',
  'org.eclipse.elk.hierarchyHandling': 'INCLUDE_CHILDREN',
  'elk.layered.nodePlacement.strategy': 'BRANDES_KOEPF',
  'elk.layered.cycleBreaking.strategy': 'DEPTH_FIRST',
  'elk.layered.crossingMinimization.strategy': 'LAYER_SWEEP',
  'elk.spacing.nodeNode': '50',
  'elk.layered.spacing.nodeNodeBetweenLayers': '100',
  'elk.layered.spacing.edgeNodeBetweenLayers': '30',
  'elk.layered.spacing.edgeEdgeBetweenLayers': '20',
  'elk.padding': '[top=40,left=40,bottom=40,right=40]',
  'org.eclipse.elk.padding': '[top=40,left=40,bottom=40,right=40]',
}

export const trafficElkLayout: LayoutStrategy = {
  layout(input: LayoutInput, deps: LayoutDeps): Promise<LayoutResult> {
    return runElkLayout(input, deps)
  },
}

async function runElkLayout(
  input: LayoutInput,
  deps: LayoutDeps,
): Promise<LayoutResult> {
  const { nodes: graphNodes, edges: graphEdges, nodeSizes } = input
  if (graphNodes.length === 0) return { nodes: [], edges: [] }

  const nodeMap = new Map(graphNodes.map((node) => [node.id, node]))
  const visibleEdges = graphEdges.filter(
    (edge) => nodeMap.has(edge.source) && nodeMap.has(edge.target),
  )
  const sizeFor = (node: GraphNode): MeasuredSize =>
    nodeSizes?.get(node.id) ?? defaultSize(node)

  const elkGraph: ElkNode = {
    id: 'topology-traffic-elk-root',
    layoutOptions: ELK_LAYOUT_OPTIONS,
    children: graphNodes.map((node) => toElkChild(node, sizeFor(node))),
    edges: visibleEdges.map(toElkEdge),
  }

  const result = await elk.layout(elkGraph)
  const positioned = new Map((result.children ?? []).map((child) => [child.id, child]))
  const routePoints = new Map(
    (result.edges ?? []).map((edge) => [edge.id, edgeRoutePoints(edge)]),
  )

  return {
    nodes: graphNodes.map((node) =>
      toReactNode(node, positioned.get(node.id), sizeFor(node), deps),
    ),
    edges: visibleEdges.map((edge) =>
      toReactEdge(edge, nodeMap, routePoints.get(edge.id)),
    ),
  }
}

function toElkChild(node: GraphNode, size: MeasuredSize): ElkNode {
  return {
    id: node.id,
    width: size.width,
    height: size.height,
    labels: [{ text: `${node.name}\n(${node.kind})`, width: size.width, height: size.height }],
  }
}

function toElkEdge(edge: GraphEdge): ElkExtendedEdge {
  return {
    id: edge.id,
    sources: [edge.source],
    targets: [edge.target],
    labels: [{ text: edge.relation, width: labelWidth(edge.relation), height: 18 }],
  }
}

function toReactNode(
  node: GraphNode,
  laidOut: ElkNode | undefined,
  _size: MeasuredSize,
  deps: LayoutDeps,
): Node {
  // Intentionally do not set `width`/`height` on the node — React Flow needs
  // to keep measuring the rendered DOM so we can feed real sizes back into ELK
  // on the second pass (see TopologyGraph measure-then-layout cycle).
  return {
    id: node.id,
    type: 'entity',
    position: {
      x: Math.round(laidOut?.x ?? 0),
      y: Math.round(laidOut?.y ?? 0),
    },
    sourcePosition: Position.Right,
    targetPosition: Position.Left,
    data: {
      ...node,
      mode: deps.mode,
    } as Record<string, unknown>,
  }
}

function toReactEdge(
  edge: GraphEdge,
  nodeMap: Map<string, GraphNode>,
  points: Array<{ x: number; y: number }> | undefined,
): Edge {
  const sourceNode = nodeMap.get(edge.source)
  const targetNode = nodeMap.get(edge.target)
  const hasRoute = points !== undefined && points.length >= 2
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
      routePoints: hasRoute ? points : undefined,
      routeStyle: hasRoute ? 'orthogonal' : undefined,
      rawElk: hasRoute ? true : undefined,
    } as Record<string, unknown>,
    ...(DIRECTED_RELATIONS.includes(edge.relation) && {
      markerEnd: { type: MarkerType.ArrowClosed, width: 12, height: 12 },
    }),
  }
}

function defaultSize(node: GraphNode): MeasuredSize {
  return {
    width: NODE_WIDTH,
    height: SMALL_KINDS.includes(node.kind) ? NODE_HEIGHT_SMALL : NODE_HEIGHT_DEFAULT,
  }
}

function labelWidth(label: string): number {
  return Math.max(48, label.length * 8)
}

function edgeRoutePoints(edge: ElkExtendedEdge): Array<{ x: number; y: number }> {
  const section = edge.sections?.[0]
  if (!section) return []
  return [section.startPoint, ...(section.bendPoints ?? []), section.endPoint].map(
    (point) => ({ x: point.x, y: point.y }),
  )
}
