package entities

type HPA struct {
	EntityID    EntityID          `json:"id"`
	ClusterName string            `json:"cluster_name"`
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Metadata    map[string]string `json:"labels,omitempty"`
}

func NewHPA(clusterName, namespace, name string, labels map[string]string) HPA {
	return HPA{
		EntityID:    ComputeEntityID(KindHPA, clusterName, namespace, name),
		ClusterName: clusterName,
		Namespace:   namespace,
		Name:        name,
		Metadata:    cloneLabels(labels),
	}
}

func (h HPA) ID() EntityID { return h.EntityID }

func (h HPA) Kind() EntityKind { return KindHPA }

func (h HPA) Labels() map[string]string { return cloneLabels(h.Metadata) }
