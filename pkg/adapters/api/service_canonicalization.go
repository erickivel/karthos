package api

import (
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/topology"
)

type serviceKey struct {
	Cluster string
	Name    string
}

func dedupeEntities(items []entities.Entity) []entities.Entity {
	if len(items) == 0 {
		return items
	}

	keepLegacy := make(map[entities.EntityID]bool, len(items))
	for _, item := range items {
		keepLegacy[item.ID()] = true
	}

	groups := make(map[serviceKey][]entities.Entity)
	for _, item := range items {
		if item.Kind() != entities.KindService {
			continue
		}
		ref := entityRefOf(item)
		key := serviceKey{Cluster: ref.Cluster, Name: ref.Name}
		groups[key] = append(groups[key], item)
	}

	for _, group := range groups {
		hasNamespaced := false
		for _, item := range group {
			if entityRefOf(item).Namespace != "" {
				hasNamespaced = true
				break
			}
		}
		if !hasNamespaced {
			continue
		}
		for _, item := range group {
			if entityRefOf(item).Namespace == "" {
				keepLegacy[item.ID()] = false
			}
		}
	}

	result := make([]entities.Entity, 0, len(items))
	for _, item := range items {
		if keepLegacy[item.ID()] {
			result = append(result, item)
		}
	}
	return result
}

func canonicalizeGraph(nodes []topology.Node, edges []topology.Edge) ([]topology.Node, []topology.Edge) {
	aliases := buildServiceAliases(nodes)

	canonicalNodes := make(map[entities.EntityID]topology.Node, len(nodes))
	for _, node := range nodes {
		canonicalID := canonicalNodeID(node.ID, aliases)
		node.ID = canonicalID

		existing, ok := canonicalNodes[canonicalID]
		if !ok || preferNode(node, existing) {
			canonicalNodes[canonicalID] = node
		}
	}

	dedupedNodes := make([]topology.Node, 0, len(canonicalNodes))
	for _, node := range canonicalNodes {
		dedupedNodes = append(dedupedNodes, node)
	}

	canonicalEdges := make(map[string]topology.Edge, len(edges))
	for _, edge := range edges {
		originalSource, originalTarget := edge.Source, edge.Target
		edge.Source = canonicalNodeID(edge.Source, aliases)
		edge.Target = canonicalNodeID(edge.Target, aliases)
		if edge.Source != originalSource || edge.Target != originalTarget {
			if edge.Relation == topology.RelRunsOn && edge.Metadata != nil && edge.Metadata["k8s.pod.uid"] != "" {
				edge.ID = topology.ComputeEdgeIDWithDiscriminator(edge.Source, edge.Target, edge.Relation, edge.Metadata["k8s.pod.uid"])
			} else {
				edge.ID = topology.ComputeEdgeID(edge.Source, edge.Target, edge.Relation)
			}
		}

		existing, ok := canonicalEdges[edge.ID]
		if !ok || edge.LastSeen.After(existing.LastSeen) {
			canonicalEdges[edge.ID] = edge
		}
	}

	dedupedEdges := make([]topology.Edge, 0, len(canonicalEdges))
	for _, edge := range canonicalEdges {
		dedupedEdges = append(dedupedEdges, edge)
	}

	return dedupedNodes, dedupedEdges
}

func buildServiceAliases(nodes []topology.Node) map[entities.EntityID]entities.EntityID {
	aliases := make(map[entities.EntityID]entities.EntityID)
	groups := make(map[serviceKey][]topology.Node)

	for _, node := range nodes {
		if node.Kind != entities.KindService {
			continue
		}
		key := serviceKey{Cluster: nodeCluster(node), Name: nodeName(node)}
		groups[key] = append(groups[key], node)
	}

	for _, group := range groups {
		var canonical *topology.Node
		for i := range group {
			if nodeNamespace(group[i]) != "" {
				if canonical == nil {
					canonical = &group[i]
					continue
				}
				canonical = nil
				break
			}
		}
		if canonical == nil {
			continue
		}
		for _, node := range group {
			if node.ID != canonical.ID && nodeNamespace(node) == "" {
				aliases[node.ID] = canonical.ID
			}
		}
	}

	return aliases
}

func canonicalNodeID(id entities.EntityID, aliases map[entities.EntityID]entities.EntityID) entities.EntityID {
	if canonical, ok := aliases[id]; ok {
		return canonical
	}
	return id
}

func preferNode(candidate, existing topology.Node) bool {
	return nodeNamespace(candidate) != "" && nodeNamespace(existing) == ""
}

func nodeName(node topology.Node) string {
	if node.Labels == nil {
		return ""
	}
	return node.Labels["name"]
}

func nodeNamespace(node topology.Node) string {
	if node.Labels == nil {
		return ""
	}
	return node.Labels["namespace"]
}

func nodeCluster(node topology.Node) string {
	if node.Labels == nil {
		return ""
	}
	return node.Labels["cluster"]
}
