package entities

type Ingress struct {
	EntityID    EntityID          `json:"id"`
	ClusterName string            `json:"cluster_name"`
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Host        string            `json:"host,omitempty"`
	Metadata    map[string]string `json:"labels,omitempty"`
}

func NewIngress(clusterName, namespace, name, host string, labels map[string]string) Ingress {
	return Ingress{
		EntityID:    ComputeEntityID(KindIngress, clusterName, namespace, name, host),
		ClusterName: clusterName,
		Namespace:   namespace,
		Name:        name,
		Host:        host,
		Metadata:    cloneLabels(labels),
	}
}

func (i Ingress) ID() EntityID { return i.EntityID }

func (i Ingress) Kind() EntityKind { return KindIngress }

func (i Ingress) Labels() map[string]string { return cloneLabels(i.Metadata) }
