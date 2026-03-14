package entities

type Container struct {
	EntityID             EntityID          `json:"id"`
	ClusterName          string            `json:"cluster_name"`
	Namespace            string            `json:"namespace"`
	Name                 string            `json:"name"`
	PodID                EntityID          `json:"pod_id"`
	CPURequestMillicores uint32            `json:"cpu_request_millicores,omitempty"`
	CPULimitMillicores   uint32            `json:"cpu_limit_millicores,omitempty"`
	MemoryRequestBytes   uint64            `json:"memory_request_bytes,omitempty"`
	MemoryLimitBytes     uint64            `json:"memory_limit_bytes,omitempty"`
	InboundBandwidthBps  uint64            `json:"inbound_bandwidth_bps,omitempty"`
	OutboundBandwidthBps uint64            `json:"outbound_bandwidth_bps,omitempty"`
	Metadata             map[string]string `json:"labels,omitempty"`
}

func NewContainer(clusterName, namespace, podName, name string, labels map[string]string) Container {
	return Container{
		EntityID:    ComputeEntityID(KindContainer, clusterName, namespace, podName, name),
		ClusterName: clusterName,
		Namespace:   namespace,
		Name:        name,
		Metadata:    cloneLabels(labels),
	}
}

func (c Container) ID() EntityID { return c.EntityID }

func (c Container) Kind() EntityKind { return KindContainer }

func (c Container) Labels() map[string]string { return cloneLabels(c.Metadata) }
