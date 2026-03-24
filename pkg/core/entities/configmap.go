package entities

type ConfigMap struct {
	EntityID    EntityID          `json:"id"`
	ClusterName string            `json:"cluster_name"`
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Metadata    map[string]string `json:"labels,omitempty"`
}

func NewConfigMap(clusterName, namespace, name string, labels map[string]string) ConfigMap {
	return ConfigMap{
		EntityID:    ComputeEntityID(KindConfigMap, clusterName, namespace, name),
		ClusterName: clusterName,
		Namespace:   namespace,
		Name:        name,
		Metadata:    cloneLabels(labels),
	}
}

func (c ConfigMap) ID() EntityID { return c.EntityID }

func (c ConfigMap) Kind() EntityKind { return KindConfigMap }

func (c ConfigMap) Labels() map[string]string { return cloneLabels(c.Metadata) }
