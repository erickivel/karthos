package entities

type Secret struct {
	EntityID    EntityID          `json:"id"`
	ClusterName string            `json:"cluster_name"`
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Metadata    map[string]string `json:"labels,omitempty"`
}

func NewSecret(clusterName, namespace, name string, labels map[string]string) Secret {
	return Secret{
		EntityID:    ComputeEntityID(KindSecret, clusterName, namespace, name),
		ClusterName: clusterName,
		Namespace:   namespace,
		Name:        name,
		Metadata:    cloneLabels(labels),
	}
}

func (s Secret) ID() EntityID { return s.EntityID }

func (s Secret) Kind() EntityKind { return KindSecret }

func (s Secret) Labels() map[string]string { return cloneLabels(s.Metadata) }
