package entities

type EntityKind string

const (
	KindCluster   EntityKind = "cluster"
	KindNode      EntityKind = "node"
	KindNamespace EntityKind = "namespace"
	KindWorkload  EntityKind = "workload"
	KindPod       EntityKind = "pod"
	KindContainer EntityKind = "container"
	KindService   EntityKind = "service"
	KindIngress   EntityKind = "ingress"
	KindEndpoint  EntityKind = "endpoint"
	KindConfigMap EntityKind = "configmap"
	KindSecret    EntityKind = "secret"
	KindPVC       EntityKind = "pvc"
	KindHPA       EntityKind = "hpa"
)

type EntityID string

// Entity is the stable graph-facing abstraction for discovered infrastructure
// and application components. It is intentionally broader than Kubernetes
// resource kinds so the topology model can also represent non-Kubernetes nodes.
type Entity interface {
	ID() EntityID
	Kind() EntityKind
	Labels() map[string]string
}

func cloneLabels(labels map[string]string) map[string]string {
	if len(labels) == 0 {
		return nil
	}

	cloned := make(map[string]string, len(labels))
	for key, value := range labels {
		cloned[key] = value
	}

	return cloned
}
