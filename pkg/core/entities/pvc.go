package entities

type PVC struct {
	EntityID    EntityID          `json:"id"`
	ClusterName string            `json:"cluster_name"`
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Metadata    map[string]string `json:"labels,omitempty"`
}

func NewPVC(clusterName, namespace, name string, labels map[string]string) PVC {
	return PVC{
		EntityID:    ComputeEntityID(KindPVC, clusterName, namespace, name),
		ClusterName: clusterName,
		Namespace:   namespace,
		Name:        name,
		Metadata:    cloneLabels(labels),
	}
}

func (p PVC) ID() EntityID { return p.EntityID }

func (p PVC) Kind() EntityKind { return KindPVC }

func (p PVC) Labels() map[string]string { return cloneLabels(p.Metadata) }
