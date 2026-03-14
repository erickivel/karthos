package entities

type Cluster struct {
	EntityID EntityID          `json:"id"`
	Name     string            `json:"name"`
	Metadata map[string]string `json:"labels,omitempty"`
}

func NewCluster(name string, labels map[string]string) Cluster {
	return Cluster{
		EntityID: ComputeEntityID(KindCluster, name),
		Name:     name,
		Metadata: cloneLabels(labels),
	}
}

func (c Cluster) ID() EntityID {
	return c.EntityID
}

func (c Cluster) Kind() EntityKind {
	return KindCluster
}

func (c Cluster) Labels() map[string]string {
	return cloneLabels(c.Metadata)
}
