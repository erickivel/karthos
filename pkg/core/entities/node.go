package entities

type Node struct {
	EntityID    EntityID          `json:"id"`
	ClusterName string            `json:"cluster_name"`
	Name        string            `json:"name"`
	Metadata    map[string]string `json:"labels,omitempty"`
}

func NewNode(clusterName, name string, labels map[string]string) Node {
	return Node{
		EntityID:    ComputeEntityID(KindNode, clusterName, name),
		ClusterName: clusterName,
		Name:        name,
		Metadata:    cloneLabels(labels),
	}
}

func (n Node) ID() EntityID { return n.EntityID }

func (n Node) Kind() EntityKind { return KindNode }

func (n Node) Labels() map[string]string { return cloneLabels(n.Metadata) }
