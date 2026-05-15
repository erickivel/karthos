package entities

type ResourceQuota struct {
	EntityID    EntityID          `json:"id"`
	ClusterName string            `json:"cluster_name"`
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Metadata    map[string]string `json:"labels,omitempty"`
}

func NewResourceQuota(clusterName, namespace, name string, labels map[string]string) ResourceQuota {
	return ResourceQuota{
		EntityID:    ComputeEntityID(KindResourceQuota, clusterName, namespace, name),
		ClusterName: clusterName,
		Namespace:   namespace,
		Name:        name,
		Metadata:    cloneLabels(labels),
	}
}

func (q ResourceQuota) ID() EntityID { return q.EntityID }

func (q ResourceQuota) Kind() EntityKind { return KindResourceQuota }

func (q ResourceQuota) Labels() map[string]string { return cloneLabels(q.Metadata) }
