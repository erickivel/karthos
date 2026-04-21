package semconv

// Resource attribute keys from OTLP semantic conventions.
const (
	ResourceK8sClusterName     = "k8s.cluster.name"
	ResourceK8sContainerName   = "k8s.container.name"
	ResourceK8sDaemonSetName   = "k8s.daemonset.name"
	ResourceK8sDeploymentName  = "k8s.deployment.name"
	ResourceK8sNamespaceName   = "k8s.namespace.name"
	ResourceK8sNodeName        = "k8s.node.name"
	ResourceK8sPodName         = "k8s.pod.name"
	ResourceK8sReplicaSetName  = "k8s.replicaset.name"
	ResourceK8sStatefulSetName = "k8s.statefulset.name"
	ResourceServiceName        = "service.name"
)

// Metric datapoint attribute keys.
const (
	AttrClient          = "client"
	AttrClientNamespace = "client_namespace"
	AttrCondition       = "condition"
	AttrDirection       = "direction"
	AttrPeerService     = "peer.service"
	AttrServer          = "server"
	AttrServerNamespace = "server_namespace"
)

// Internal labels derived during normalization.
const (
	LabelEdgeClient   = "edge_client"
	LabelEdgeClientID = "edge_client_id"
	LabelEdgeServer   = "edge_server"
	LabelEdgeServerID = "edge_server_id"
	LabelSource       = "source"
)
