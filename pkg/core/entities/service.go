package entities

type Service struct {
	EntityID    EntityID          `json:"id"`
	ClusterName string            `json:"cluster_name"`
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Metadata    map[string]string `json:"labels,omitempty"`
}

func NewService(clusterName, namespace, name string, labels map[string]string) Service {
	return Service{
		EntityID:    ComputeEntityID(KindService, clusterName, namespace, name),
		ClusterName: clusterName,
		Namespace:   namespace,
		Name:        name,
		Metadata:    cloneLabels(labels),
	}
}

func (s Service) ID() EntityID { return s.EntityID }

func (s Service) Kind() EntityKind { return KindService }

func (s Service) Labels() map[string]string { return cloneLabels(s.Metadata) }
