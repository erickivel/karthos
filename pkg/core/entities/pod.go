package entities

type Pod struct {
	EntityID    EntityID          `json:"id"`
	ClusterName string            `json:"cluster_name"`
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Metadata    map[string]string `json:"labels,omitempty"`
}

func NewPod(clusterName, namespace, name string, labels map[string]string) Pod {
	return Pod{
		EntityID:    ComputeEntityID(KindPod, clusterName, namespace, name),
		ClusterName: clusterName,
		Namespace:   namespace,
		Name:        name,
		Metadata:    cloneLabels(labels),
	}
}

func (p Pod) ID() EntityID { return p.EntityID }

func (p Pod) Kind() EntityKind { return KindPod }

func (p Pod) Labels() map[string]string { return cloneLabels(p.Metadata) }
