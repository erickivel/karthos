package entities

type WorkloadType string

const (
	WorkloadDeployment  WorkloadType = "deployment"
	WorkloadStatefulSet WorkloadType = "statefulset"
	WorkloadDaemonSet   WorkloadType = "daemonset"
	WorkloadJob         WorkloadType = "job"
)

type Workload struct {
	EntityID    EntityID          `json:"id"`
	ClusterName string            `json:"cluster_name"`
	Namespace   string            `json:"namespace"`
	Name        string            `json:"name"`
	Type        WorkloadType      `json:"type"`
	Replicas    int32             `json:"replicas,omitempty"`
	Metadata    map[string]string `json:"labels,omitempty"`
}

func NewWorkload(clusterName, namespace, name string, workloadType WorkloadType, labels map[string]string) Workload {
	return Workload{
		EntityID:    ComputeEntityID(KindWorkload, clusterName, namespace, string(workloadType), name),
		ClusterName: clusterName,
		Namespace:   namespace,
		Name:        name,
		Type:        workloadType,
		Metadata:    cloneLabels(labels),
	}
}

func (w Workload) ID() EntityID { return w.EntityID }

func (w Workload) Kind() EntityKind { return KindWorkload }

func (w Workload) Labels() map[string]string { return cloneLabels(w.Metadata) }
