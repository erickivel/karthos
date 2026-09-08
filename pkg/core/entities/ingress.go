package entities

type Ingress struct {
	EntityID    EntityID          `json:"id"`
	ClusterName string            `json:"cluster_name"`
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Metadata    map[string]string `json:"labels,omitempty"`
}

func NewIngress(clusterName, namespace, name string, labels map[string]string) Ingress {
	return Ingress{
		EntityID:    ComputeEntityID(KindIngress, clusterName, namespace, name),
		ClusterName: clusterName,
		Namespace:   namespace,
		Name:        name,
		Metadata:    cloneLabels(labels),
	}
}

func (i Ingress) ID() EntityID { return i.EntityID }

func (i Ingress) Kind() EntityKind { return KindIngress }

func (i Ingress) Labels() map[string]string { return cloneLabels(i.Metadata) }
