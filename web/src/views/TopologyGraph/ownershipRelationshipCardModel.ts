import type { GraphEdge, GraphNode } from '@/types/graph'

const DEPENDENCY_RELATIONS = new Set(['uses', 'mounts'])

export type RelationshipDirection = 'inbound' | 'outbound'

export interface RelationshipEntry {
  id: string
  relation: 'uses' | 'mounts'
  direction: RelationshipDirection
  related: GraphNode
}

export function relationshipEntriesFor(
  nodes: GraphNode[],
  edges: GraphEdge[],
  entityID: string,
): RelationshipEntry[] {
  const nodeByID = new Map(nodes.map((node) => [node.id, node]))

  return edges.reduce<RelationshipEntry[]>((entries, edge) => {
    if (!isDependencyRelation(edge.relation)) return entries

    if (edge.source === entityID) {
      const related = nodeByID.get(edge.target)
      if (related) entries.push({ id: edge.id, relation: edge.relation, direction: 'outbound', related })
      return entries
    }
    if (edge.target === entityID) {
      const related = nodeByID.get(edge.source)
      if (related) entries.push({ id: edge.id, relation: edge.relation, direction: 'inbound', related })
      return entries
    }
    return entries
  }, [])
}

function isDependencyRelation(relation: GraphEdge['relation']): relation is RelationshipEntry['relation'] {
  return DEPENDENCY_RELATIONS.has(relation)
}
