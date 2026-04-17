import { useMemo } from 'react'
import dagre from '@dagrejs/dagre'
import { MarkerType, type Node, type Edge } from '@xyflow/react'
import type { GraphNode, GraphEdge, EntityKind, RelationType } from '@/types/graph'
import type { NodeMetric } from '@/hooks/useNodeMetrics'
import type { EdgeMetricSnapshot, GraphEdgeMetricData } from './edgeMetrics'
import { getEdgeMetricKeyFromNodes } from './edgeMetrics'

const DIRECTED_RELATIONS: RelationType[] = ['calls', 'runs_on', 'routes_to', 'exposes']

const NODE_WIDTH = 240
const NODE_HEIGHT_DEFAULT = 170
const NODE_HEIGHT_SMALL = 90
const NS_PADDING = 30
const NS_PILL_OFFSET = 24 // extra top padding for the pill label

const SMALL_KINDS: EntityKind[] = ['configmap', 'secret', 'pvc']

interface LayoutResult {
  nodes: Node[]
  edges: Edge[]
}

export function useGraphLayout(
  graphNodes: GraphNode[],
  graphEdges: GraphEdge[],
  nodeMetrics?: Map<string, NodeMetric>,
  edgeMetrics?: Map<string, EdgeMetricSnapshot>,
): LayoutResult {
  return useMemo(() => {
    if (graphNodes.length === 0) return { nodes: [], edges: [] }

    // Separate namespace nodes from content nodes
    const namespaceNodes = graphNodes.filter((n) => n.kind === 'namespace')
    const contentNodes = graphNodes.filter((n) => n.kind !== 'namespace')
    const namespaceIds = new Set(namespaceNodes.map((n) => n.id))
    const contentNodeMap = new Map(contentNodes.map((node) => [node.id, node]))

    // Remove edges that connect to/from namespace nodes
    const contentEdges = graphEdges.filter(
      (e) => !namespaceIds.has(e.source) && !namespaceIds.has(e.target),
    )

    // Use dagre compound graph to group nodes by namespace
    const g = new dagre.graphlib.Graph({ compound: true })
    g.setDefaultEdgeLabel(() => ({}))
    g.setGraph({ rankdir: 'TB', ranksep: 80, nodesep: 40 })

    // Create cluster nodes for each namespace
    const nsNameToNode = new Map<string, GraphNode>()
    for (const nsNode of namespaceNodes) {
      nsNameToNode.set(nsNode.name, nsNode)
      g.setNode(`ns:${nsNode.name}`, {})
    }

    // Add content nodes and assign to namespace clusters
    for (const node of contentNodes) {
      const height = SMALL_KINDS.includes(node.kind) ? NODE_HEIGHT_SMALL : NODE_HEIGHT_DEFAULT
      g.setNode(node.id, { width: NODE_WIDTH, height })
      if (node.namespace && nsNameToNode.has(node.namespace)) {
        g.setParent(node.id, `ns:${node.namespace}`)
      }
    }

    // Add content edges
    for (const edge of contentEdges) {
      if (g.hasNode(edge.source) && g.hasNode(edge.target)) {
        g.setEdge(edge.source, edge.target)
      }
    }

    dagre.layout(g)

    // Build positioned content nodes
    const positionedContent: Node[] = contentNodes.map((node) => {
      const pos = g.node(node.id)
      const h = pos?.height ?? NODE_HEIGHT_DEFAULT
      const metrics = nodeMetrics?.get(node.id)
      const labels = { ...node.labels }
      if (metrics?.cpu !== undefined) labels['cpu_usage_millicores'] = String(metrics.cpu)
      if (metrics?.mem !== undefined) labels['memory_usage_bytes'] = String(metrics.mem)
      return {
        id: node.id,
        type: 'entity' as const,
        position: { x: (pos?.x ?? 0) - NODE_WIDTH / 2, y: (pos?.y ?? 0) - h / 2 },
        data: { ...node, labels } as Record<string, unknown>,
      }
    })

    // Compute bounding boxes per namespace from positioned content nodes
    const nsBounds = new Map<string, { minX: number; minY: number; maxX: number; maxY: number }>()

    for (const rfNode of positionedContent) {
      const graphNode = contentNodes.find((n) => n.id === rfNode.id)
      const ns = graphNode?.namespace
      if (!ns) continue

      const dagreNode = g.node(rfNode.id)
      const w = NODE_WIDTH
      const h = dagreNode?.height ?? NODE_HEIGHT_DEFAULT
      const x = rfNode.position.x
      const y = rfNode.position.y

      const bounds = nsBounds.get(ns)
      if (bounds) {
        bounds.minX = Math.min(bounds.minX, x)
        bounds.minY = Math.min(bounds.minY, y)
        bounds.maxX = Math.max(bounds.maxX, x + w)
        bounds.maxY = Math.max(bounds.maxY, y + h)
      } else {
        nsBounds.set(ns, { minX: x, minY: y, maxX: x + w, maxY: y + h })
      }
    }

    // Create namespace container nodes from bounding boxes
    const nsContainerNodes: Node[] = namespaceNodes
      .filter((nsNode) => nsBounds.has(nsNode.name))
      .map((nsNode) => {
        const bounds = nsBounds.get(nsNode.name)!
        return {
          id: nsNode.id,
          type: 'namespace' as const,
          position: {
            x: bounds.minX - NS_PADDING,
            y: bounds.minY - NS_PADDING - NS_PILL_OFFSET,
          },
          zIndex: -1,
          data: {
            ...nsNode,
            nsWidth: bounds.maxX - bounds.minX + NS_PADDING * 2,
            nsHeight: bounds.maxY - bounds.minY + NS_PADDING * 2 + NS_PILL_OFFSET,
          } as Record<string, unknown>,
          selectable: true,
          draggable: false,
        }
      })

    const nodes = [...nsContainerNodes, ...positionedContent]

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
  }, [graphNodes, graphEdges, nodeMetrics, edgeMetrics])
}
