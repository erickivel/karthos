package entities

type WorkloadType string

const (
	WorkloadTypeLabel                          = "k8s.workload.type"
	WorkloadDeployment            WorkloadType = "deployment"
	WorkloadStatefulSet           WorkloadType = "statefulset"
	WorkloadDaemonSet             WorkloadType = "daemonset"
	WorkloadReplicaSet            WorkloadType = "replicaset"
	WorkloadJob                   WorkloadType = "job"
	WorkloadCronJob               WorkloadType = "cronjob"
	WorkloadReplicationController WorkloadType = "replication_controller"
)

type Workload struct {
	EntityID          EntityID          `json:"id"`
	ClusterName       string            `json:"cluster_name"`
	Namespace         string            `json:"namespace"`
	Name              string            `json:"name"`
	Type              WorkloadType      `json:"type"`
	Replicas          int32             `json:"replicas,omitempty"`
	Metadata          map[string]string `json:"labels,omitempty"`
	PodTemplateLabels map[string]string `json:"pod_template_labels,omitempty"`
	ConfigMapRefs     []string          `json:"configmap_refs,omitempty"`
	SecretRefs        []string          `json:"secret_refs,omitempty"`
	PVCRefs           []string          `json:"pvc_refs,omitempty"`
}

func NewWorkload(clusterName, namespace, name string, workloadType WorkloadType, labels map[string]string) Workload {
	metadata := cloneLabels(labels)
	if metadata == nil {
		metadata = make(map[string]string, 1)
	}
	metadata[WorkloadTypeLabel] = string(workloadType)
	return Workload{
		EntityID:    ComputeEntityID(KindWorkload, clusterName, namespace, string(workloadType), name),
		ClusterName: clusterName,
		Namespace:   namespace,
		Name:        name,
		Type:        workloadType,
		Metadata:    metadata,
	}
}

func (w Workload) ID() EntityID { return w.EntityID }

func (w Workload) Kind() EntityKind { return KindWorkload }

func (w Workload) Labels() map[string]string { return cloneLabels(w.Metadata) }
