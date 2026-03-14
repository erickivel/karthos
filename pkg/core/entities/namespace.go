package entities

type Namespace struct {
	EntityID    EntityID          `json:"id"`
	ClusterName string            `json:"cluster_name"`
	Name        string            `json:"name"`
	Metadata    map[string]string `json:"labels,omitempty"`
}

func NewNamespace(clusterName, name string, labels map[string]string) Namespace {
	return Namespace{
		EntityID:    ComputeEntityID(KindNamespace, clusterName, name),
		ClusterName: clusterName,
		Name:        name,
		Metadata:    cloneLabels(labels),
	}
}

func (n Namespace) ID() EntityID { return n.EntityID }

func (n Namespace) Kind() EntityKind { return KindNamespace }

func (n Namespace) Labels() map[string]string { return cloneLabels(n.Metadata) }
