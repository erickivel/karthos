package semconv

// Resource attribute keys from OTLP semantic conventions.
const (
	ResourceK8sClusterName               = "k8s.cluster.name"
	ResourceK8sContainerName             = "k8s.container.name"
	ResourceK8sCronJobName               = "k8s.cronjob.name"
	ResourceK8sDaemonSetName             = "k8s.daemonset.name"
	ResourceK8sDeploymentName            = "k8s.deployment.name"
	ResourceK8sHPAName                   = "k8s.hpa.name"
	ResourceK8sJobName                   = "k8s.job.name"
	ResourceK8sNamespaceName             = "k8s.namespace.name"
	ResourceK8sNodeName                  = "k8s.node.name"
	ResourceK8sPodName                   = "k8s.pod.name"
	ResourceK8sReplicationControllerName = "k8s.replicationcontroller.name"
	ResourceK8sReplicaSetName            = "k8s.replicaset.name"
	ResourceK8sResourceQuotaName         = "k8s.resourcequota.name"
	ResourceK8sStatefulSetName           = "k8s.statefulset.name"
	ResourceServiceName                  = "service.name"
)

// Metric datapoint attribute keys.
const (
	AttrClient                   = "client"
	AttrClientNamespace          = "client_namespace"
	AttrCondition                = "condition"
	AttrDirection                = "direction"
	AttrInterface                = "interface"
	AttrGRPCStatusCode           = "rpc.grpc.status_code"
	AttrHTTPStatusCode           = "http.response.status_code"
	AttrK8sPVCName               = "k8s.persistentvolumeclaim.name"
	AttrK8sVolumeName            = "k8s.volume.name"
	AttrK8sVolumeType            = "k8s.volume.type"
	AttrMeshID                   = "mesh.id"
	AttrMeshPolicy               = "mesh.security.policy"
	AttrMeshResponseFlags        = "mesh.envoy.response_flags"
	AttrMeshSourcePrincipal      = "mesh.peer.source.principal"
	AttrMeshDestinationPrincipal = "mesh.peer.destination.principal"
	AttrNetworkProtocolName      = "network.protocol.name"
	AttrOKSGSource               = "oksg.source"
	AttrPeerService              = "peer.service"
	AttrResource                 = "resource"
	AttrServer                   = "server"
	AttrServerNamespace          = "server_namespace"
)

// Prometheus-safe datapoint attribute aliases used by scrape relabel configs.
const (
	AttrGRPCStatusCodeProm           = "rpc_grpc_status_code"
	AttrHTTPStatusCodeProm           = "http_response_status_code"
	AttrMeshIDProm                   = "mesh_id"
	AttrMeshPolicyProm               = "mesh_security_policy"
	AttrMeshResponseFlagsProm        = "mesh_envoy_response_flags"
	AttrMeshSourcePrincipalProm      = "mesh_peer_source_principal"
	AttrMeshDestinationPrincipalProm = "mesh_peer_destination_principal"
	AttrNetworkProtocolNameProm      = "network_protocol_name"
	AttrOKSGSourceProm               = "oksg_source"
)

// Internal labels derived during normalization.
const (
	LabelEdgeClient           = "edge_client"
	LabelEdgeClientID         = "edge_client_id"
	LabelEdgeServer           = "edge_server"
	LabelEdgeServerID         = "edge_server_id"
	LabelNetworkInterfaceName = "network_interface_name"
	LabelPVCName              = "persistentvolumeclaim_name"
	LabelSource               = "source"
	LabelVolumeName           = "volume_name"
	LabelVolumeType           = "volume_type"
)
