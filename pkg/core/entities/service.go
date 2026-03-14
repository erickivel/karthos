package entities

type Service struct {
	EntityID    EntityID          `json:"id"`
	ClusterName string            `json:"cluster_name"`
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Selector    map[string]string `json:"selector,omitempty"`
	Metadata    map[string]string `json:"labels,omitempty"`
}

func NewService(clusterName, namespace, name string, selector, labels map[string]string) Service {
	return Service{
		EntityID:    ComputeEntityID(KindService, clusterName, namespace, name),
		ClusterName: clusterName,
		Namespace:   namespace,
		Name:        name,
		Selector:    cloneLabels(selector),
		Metadata:    cloneLabels(labels),
	}
}

func (s Service) ID() EntityID { return s.EntityID }

func (s Service) Kind() EntityKind { return KindService }

func (s Service) Labels() map[string]string { return cloneLabels(s.Metadata) }
