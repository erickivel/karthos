package metrics

import "github.com/erickivel/oksg/pkg/core/entities"

var defaultCatalog = []CanonicalMetric{
	{Name: "k8s.pod.cpu.usage", DisplayName: "Pod CPU Usage", Unit: UnitMillicores, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed pod CPU usage.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.pod.memory.working_set", DisplayName: "Pod Memory Working Set", Unit: UnitBytes, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed pod memory working set.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.node.cpu.usage", DisplayName: "Node CPU Usage", Unit: UnitMillicores, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed node CPU usage from kubeletstats.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.node.memory.working_set", DisplayName: "Node Memory Working Set", Unit: UnitBytes, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed node memory working set from kubeletstats.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "system.cpu.utilization", DisplayName: "Host CPU Utilization", Unit: UnitPercent, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed host CPU utilization from hostmetrics, 0–100% of total host CPU.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.memory.usage", DisplayName: "Host Memory Usage", Unit: UnitBytes, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed host memory usage from hostmetrics.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.disk.io.read", DisplayName: "Disk Read Throughput", Unit: UnitBytesPerSecond, Aggregation: windowRateCounterAggregation(UnitBytes, UnitBytesPerSecond), Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed disk read throughput per entity.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.disk.io.write", DisplayName: "Disk Write Throughput", Unit: UnitBytesPerSecond, Aggregation: windowRateCounterAggregation(UnitBytes, UnitBytesPerSecond), Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed disk write throughput per entity.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "http.server.request.count", DisplayName: "HTTP Request Rate", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEntity}, Description: "Served as a 1-minute request rate for an HTTP service.", Sources: []SourceRef{{ID: SourcePrometheusApp, Priority: 1}}},
	{Name: "rpc.server.request.count", DisplayName: "RPC Request Rate", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEntity}, Description: "Served as a 1-minute request rate for an RPC service.", Sources: []SourceRef{{ID: SourceTracesSpanmetrics, Priority: 1}}},
	{Name: "servicegraph.request.count", DisplayName: "Service Graph Request Rate", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEntity, ScopeEdge}, Description: "Served as a 1-minute request rate for a service or edge.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}, {ID: SourceTracesServicegraph, Priority: 2}}},
	{Name: "http.server.request.duration", DisplayName: "HTTP Request Duration", Unit: UnitNanoseconds, Aggregation: windowDerivedAggregation(AggregationWindowMean, UnitNanoseconds), Category: CategoryLatency, Scopes: []MetricScope{ScopeEntity}, Description: "Observed HTTP request duration.", Sources: []SourceRef{{ID: SourcePrometheusApp, Priority: 1}}},
	{Name: "rpc.server.duration", DisplayName: "RPC Request Duration", Unit: UnitNanoseconds, Aggregation: windowDerivedAggregation(AggregationWindowMean, UnitNanoseconds), Category: CategoryLatency, Scopes: []MetricScope{ScopeEntity}, Description: "Observed RPC request duration.", Sources: []SourceRef{{ID: SourceTracesSpanmetrics, Priority: 1}}},
	{Name: "servicegraph.request.duration", DisplayName: "Service Graph Request Duration", Unit: UnitNanoseconds, Aggregation: windowDerivedAggregation(AggregationWindowMean, UnitNanoseconds), Category: CategoryLatency, Scopes: []MetricScope{ScopeEdge}, Description: "Observed request latency between graph edges.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}, {ID: SourceTracesServicegraph, Priority: 2}}},
	{Name: "servicegraph.request.error.count", DisplayName: "Service Graph Error Rate", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryError, Scopes: []MetricScope{ScopeEntity, ScopeEdge}, Description: "Served as a 1-minute error rate for a service or edge.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}, {ID: SourceTracesServicegraph, Priority: 2}}},
	{Name: "servicegraph.request.error.ratio", DisplayName: "Service Graph Error Ratio", Unit: UnitRatio, Aggregation: windowDerivedAggregation(AggregationWindowRatio, UnitRatio), Category: CategoryError, Scopes: []MetricScope{ScopeEntity, ScopeEdge}, Description: "Observed error ratio for a service or edge."},
	{Name: "servicegraph.request.body.size", DisplayName: "Service Graph Request Body Size", Unit: UnitBytes, Aggregation: windowDerivedAggregation(AggregationWindowMean, UnitBytes), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEdge}, Description: "Observed request body size between graph edges.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}}},
	{Name: "servicegraph.response.body.size", DisplayName: "Service Graph Response Body Size", Unit: UnitBytes, Aggregation: windowDerivedAggregation(AggregationWindowMean, UnitBytes), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEdge}, Description: "Observed response body size between graph edges.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}}},
	{Name: "servicegraph.grpc.request.message.count", DisplayName: "Service Graph gRPC Request Message Rate", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEdge}, Description: "Observed gRPC request message rate between graph edges.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}}},
	{Name: "servicegraph.grpc.response.message.count", DisplayName: "Service Graph gRPC Response Message Rate", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEdge}, Description: "Observed gRPC response message rate between graph edges.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}}},
	{Name: "servicegraph.tcp.connection.open.count", DisplayName: "Service Graph TCP Open Connection Rate", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEdge}, Description: "Observed TCP opened connection rate between graph edges.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}}},
	{Name: "servicegraph.tcp.connection.close.count", DisplayName: "Service Graph TCP Closed Connection Rate", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEdge}, Description: "Observed TCP closed connection rate between graph edges.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}}},
	{Name: "k8s.container.cpu.request", DisplayName: "Container CPU Request", Unit: UnitMillicores, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Requested CPU for a container.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.container.cpu.limit", DisplayName: "Container CPU Limit", Unit: UnitMillicores, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "CPU limit for a container.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.container.memory.request", DisplayName: "Container Memory Request", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Requested memory for a container.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.container.memory.limit", DisplayName: "Container Memory Limit", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Memory limit for a container.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.container.cpu.usage", DisplayName: "Container CPU Usage", Unit: UnitMillicores, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed container CPU usage.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.container.memory.usage", DisplayName: "Container Memory Usage", Unit: UnitBytes, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed container memory usage.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.container.ready", DisplayName: "Container Ready", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Container Ready condition (1=true, 0=false).", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.container.restarts", DisplayName: "Container Restarts", Unit: UnitCount, Aggregation: latestCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Cumulative container restart count.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.replicaset.desired", DisplayName: "ReplicaSet Desired Replicas", Unit: UnitCount, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Desired replica count for a replicaset.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "system.network.io.receive", DisplayName: "Network Receive Throughput", Unit: UnitBytesPerSecond, Aggregation: windowRateCounterAggregation(UnitBytes, UnitBytesPerSecond), Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Network receive throughput per entity.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.network.io.transmit", DisplayName: "Network Transmit Throughput", Unit: UnitBytesPerSecond, Aggregation: windowRateCounterAggregation(UnitBytes, UnitBytesPerSecond), Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Network transmit throughput per entity.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.network.errors", DisplayName: "Network Errors", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryError, Scopes: []MetricScope{ScopeEntity}, Description: "Network interface error rate per entity.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.network.dropped", DisplayName: "Network Dropped", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryError, Scopes: []MetricScope{ScopeEntity}, Description: "Network interface dropped packet rate per entity.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.network.packets", DisplayName: "Network Packet Rate", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEntity}, Description: "Network interface packet rate per entity.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.disk.operations", DisplayName: "Disk Operations", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Disk I/O operation rate per entity.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.disk.operation_time", DisplayName: "Disk Operation Time", Unit: UnitNanoseconds, Aggregation: windowRateCounterAggregation(UnitNanoseconds, UnitNanosecondsPerSecond), Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Cumulative time spent on disk I/O operations per entity.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "k8s.volume.available", DisplayName: "Volume Available Bytes", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Available bytes on a pod volume; PVC-backed volumes attach to the PVC entity.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.volume.capacity", DisplayName: "Volume Capacity", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Total capacity bytes of a pod volume; PVC-backed volumes attach to the PVC entity.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.volume.inodes.free", DisplayName: "Volume Inodes Free", Unit: UnitCount, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Free inodes on a pod volume; PVC-backed volumes attach to the PVC entity.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.volume.inodes.used", DisplayName: "Volume Inodes Used", Unit: UnitCount, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Used inodes on a pod volume; PVC-backed volumes attach to the PVC entity.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "system.filesystem.usage", DisplayName: "Filesystem Usage", Unit: UnitBytes, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Filesystem usage in bytes per mount point.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.filesystem.utilization", DisplayName: "Filesystem Utilization", Unit: UnitPercent, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Filesystem utilization per mount point, 0–100% of capacity.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.cpu.load_average.1m", DisplayName: "Load Average (1m)", Unit: UnitCount, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "1-minute CPU load average.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.cpu.load_average.5m", DisplayName: "Load Average (5m)", Unit: UnitCount, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "5-minute CPU load average.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.cpu.load_average.15m", DisplayName: "Load Average (15m)", Unit: UnitCount, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "15-minute CPU load average.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.paging.usage", DisplayName: "Paging Usage", Unit: UnitBytes, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Paging or swap usage in bytes.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.paging.operations", DisplayName: "Paging Operations", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Paging operations per second.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "k8s.namespace.phase", DisplayName: "Namespace Phase", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Namespace phase (1=active, 0=terminating).", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.pod.phase", DisplayName: "Pod Phase", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Pod lifecycle phase, integer-encoded: 1=Pending, 2=Running, 3=Succeeded, 4=Failed, 5=Unknown.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.pod.status_reason", DisplayName: "Pod Status Reason", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Pod failure reason, integer-encoded: 1=Evicted, 2=NodeAffinity, 3=NodeLost, 4=Shutdown, 5=UnexpectedAdmissionError, 6=Unknown.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "oksg.pod.health.status", DisplayName: "Pod Health", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Composite pod health status. 1=healthy (phase=Running), 4=pending, 5=succeeded, 6=failed, 7=unknown. Derived from k8s.pod.phase. running_not_ready (2) and restarting (3) are observable at container scope via oksg.container.health.status — rolling them up to pod scope requires a pod_id label on container metrics and is a follow-up."},
	{Name: "oksg.container.health.status", DisplayName: "Container Health", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Composite container health status. 1=healthy (ready, no recent restart), 2=running_not_ready, 3=restarting (any restart in last 5m). Derived from k8s.container.ready + rate(k8s.container.restarts[5m])."},
	{Name: "k8s.node.condition.ready", DisplayName: "Node Ready", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Node Ready condition (1=true, 0=false).", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.node.condition.memory_pressure", DisplayName: "Node Memory Pressure", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Node MemoryPressure condition (1=true, 0=false).", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.node.condition.disk_pressure", DisplayName: "Node Disk Pressure", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Node DiskPressure condition (1=true, 0=false).", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.node.allocatable_cpu", DisplayName: "Node Allocatable CPU", Unit: UnitMillicores, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Node allocatable CPU in millicores.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.node.allocatable_memory", DisplayName: "Node Allocatable Memory", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Node allocatable memory in bytes.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.deployment.available", DisplayName: "Deployment Available Replicas", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Available replicas for a deployment.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.deployment.desired", DisplayName: "Deployment Desired Replicas", Unit: UnitCount, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Desired replicas for a deployment.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.daemonset.desired_scheduled_nodes", DisplayName: "DaemonSet Desired Nodes", Unit: UnitCount, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Nodes that should run the daemon pod.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.daemonset.ready_nodes", DisplayName: "DaemonSet Ready Nodes", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Nodes with ready daemon pod instances.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.statefulset.desired_pods", DisplayName: "StatefulSet Desired Pods", Unit: UnitCount, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Desired pod replicas for a statefulset.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.statefulset.ready_pods", DisplayName: "StatefulSet Ready Pods", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Ready pods for a statefulset.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},

	// Istio TCP byte throughput (cumulative bytes; rate()-derived at query time).
	{Name: "servicegraph.tcp.bytes.sent", DisplayName: "Service Graph TCP Bytes Sent", Unit: UnitBytesPerSecond, Aggregation: windowRateCounterAggregation(UnitBytes, UnitBytesPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEdge}, Description: "Observed TCP bytes-sent rate between graph edges.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}}},
	{Name: "servicegraph.tcp.bytes.received", DisplayName: "Service Graph TCP Bytes Received", Unit: UnitBytesPerSecond, Aggregation: windowRateCounterAggregation(UnitBytes, UnitBytesPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEdge}, Description: "Observed TCP bytes-received rate between graph edges.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}}},

	// Kubelet pod & node network throughput / errors.
	{Name: "k8s.pod.network.io", DisplayName: "Pod Network Throughput", Unit: UnitBytesPerSecond, Aggregation: windowRateCounterAggregation(UnitBytes, UnitBytesPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEntity}, Description: "Observed pod network throughput from kubeletstats. Direction (receive/transmit) is carried in the data point's `direction` attribute.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.pod.network.errors", DisplayName: "Pod Network Errors", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryError, Scopes: []MetricScope{ScopeEntity}, Description: "Observed pod network error rate from kubeletstats.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.node.network.io", DisplayName: "Node Network Throughput (Kubelet)", Unit: UnitBytesPerSecond, Aggregation: windowRateCounterAggregation(UnitBytes, UnitBytesPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEntity}, Description: "Observed node network throughput from kubeletstats.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.node.network.errors", DisplayName: "Node Network Errors (Kubelet)", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryError, Scopes: []MetricScope{ScopeEntity}, Description: "Observed node network error rate from kubeletstats.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},

	// Kubelet filesystem at pod / node / container scopes.
	{Name: "k8s.pod.filesystem.usage", DisplayName: "Pod Filesystem Usage", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Observed pod-scope filesystem usage from kubeletstats.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.pod.filesystem.capacity", DisplayName: "Pod Filesystem Capacity", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Observed pod-scope filesystem capacity from kubeletstats.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.pod.filesystem.available", DisplayName: "Pod Filesystem Available", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Observed pod-scope filesystem available bytes from kubeletstats.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.node.filesystem.usage", DisplayName: "Node Filesystem Usage (Kubelet)", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Observed node-scope filesystem usage from kubeletstats.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.node.filesystem.capacity", DisplayName: "Node Filesystem Capacity (Kubelet)", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Observed node-scope filesystem capacity from kubeletstats.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.node.filesystem.available", DisplayName: "Node Filesystem Available (Kubelet)", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Observed node-scope filesystem available bytes from kubeletstats.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.container.filesystem.usage", DisplayName: "Container Filesystem Usage", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Observed container-scope filesystem usage from kubeletstats.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.container.filesystem.capacity", DisplayName: "Container Filesystem Capacity", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Observed container-scope filesystem capacity from kubeletstats.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.container.filesystem.available", DisplayName: "Container Filesystem Available", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Observed container-scope filesystem available bytes from kubeletstats.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},

	// Volume inodes total (free/used already cataloged above).
	{Name: "k8s.volume.inodes", DisplayName: "Volume Inodes Total", Unit: UnitCount, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Total inodes on a pod volume; PVC-backed volumes attach to the PVC entity.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},

	// k8s_cluster ephemeral & storage container resources + node allocatable ephemeral.
	{Name: "k8s.node.allocatable_ephemeral_storage", DisplayName: "Node Allocatable Ephemeral Storage", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Node allocatable ephemeral storage in bytes.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.node.allocatable_pods", DisplayName: "Node Allocatable Pods", Unit: UnitCount, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Node allocatable pod capacity.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},

	// k8s_cluster workload/controller metrics.
	{Name: "k8s.container.ephemeral_storage.request", DisplayName: "Container Ephemeral Storage Request", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Requested ephemeral storage for a container.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.container.ephemeral_storage.limit", DisplayName: "Container Ephemeral Storage Limit", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Ephemeral storage limit for a container.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.cronjob.active_jobs", DisplayName: "CronJob Active Jobs", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Actively running jobs for a cronjob.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.daemonset.current_scheduled_nodes", DisplayName: "DaemonSet Current Scheduled Nodes", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Nodes currently running the daemon pod and expected to run it.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.daemonset.misscheduled_nodes", DisplayName: "DaemonSet Misscheduled Nodes", Unit: UnitCount, Category: CategoryError, Scopes: []MetricScope{ScopeEntity}, Description: "Nodes running the daemon pod but not expected to run it.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.hpa.max_replicas", DisplayName: "HPA Max Replicas", Unit: UnitCount, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Maximum replicas configured for a horizontal pod autoscaler.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.hpa.min_replicas", DisplayName: "HPA Min Replicas", Unit: UnitCount, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Minimum replicas configured for a horizontal pod autoscaler.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.hpa.current_replicas", DisplayName: "HPA Current Replicas", Unit: UnitCount, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Current replicas managed by a horizontal pod autoscaler.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.hpa.desired_replicas", DisplayName: "HPA Desired Replicas", Unit: UnitCount, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Desired replicas managed by a horizontal pod autoscaler.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.job.active_pods", DisplayName: "Job Active Pods", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Actively running pods for a job.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.job.desired_successful_pods", DisplayName: "Job Desired Successful Pods", Unit: UnitCount, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Desired successfully completed pods for a job.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.job.failed_pods", DisplayName: "Job Failed Pods", Unit: UnitCount, Category: CategoryError, Scopes: []MetricScope{ScopeEntity}, Description: "Pods that reached Failed phase for a job.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.job.max_parallel_pods", DisplayName: "Job Max Parallel Pods", Unit: UnitCount, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Maximum desired parallel pods for a job.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.job.successful_pods", DisplayName: "Job Successful Pods", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Pods that reached Succeeded phase for a job.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.replicaset.available", DisplayName: "ReplicaSet Available Replicas", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Available pods targeted by a replicaset.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.replication_controller.desired", DisplayName: "ReplicationController Desired Replicas", Unit: UnitCount, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Desired pods for a replication controller.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.replication_controller.available", DisplayName: "ReplicationController Available Replicas", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Available pods targeted by a replication controller.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.resource_quota.hard_limit", DisplayName: "ResourceQuota Hard Limit", Unit: UnitResource, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Configured hard limit for a resource quota resource. Sample units are derived from the `resource` label when possible.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.resource_quota.used", DisplayName: "ResourceQuota Used", Unit: UnitResource, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Current usage for a resource quota resource. Sample units are derived from the `resource` label when possible.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.statefulset.current_pods", DisplayName: "StatefulSet Current Pods", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Pods created by the StatefulSet controller from the current StatefulSet version.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.statefulset.updated_pods", DisplayName: "StatefulSet Updated Pods", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Pods created by the StatefulSet controller from the updated StatefulSet version.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},

	// Hostmetrics: CPU time (cumulative seconds → ns to mirror system.disk.operation_time pattern).
	{Name: "system.cpu.time", DisplayName: "Host CPU Time", Unit: UnitNanoseconds, Aggregation: windowRateCounterAggregation(UnitNanoseconds, UnitNanosecondsPerSecond), Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Cumulative host CPU time per CPU/state from hostmetrics.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},

	// Hostmetrics: disk saturation.
	{Name: "system.disk.io_time", DisplayName: "Disk Active Time", Unit: UnitNanoseconds, Aggregation: windowRateCounterAggregation(UnitNanoseconds, UnitNanosecondsPerSecond), Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Cumulative time the disk spent active (active-seconds → ns).", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.disk.weighted_io_time", DisplayName: "Disk Weighted Active Time", Unit: UnitNanoseconds, Aggregation: windowRateCounterAggregation(UnitNanoseconds, UnitNanosecondsPerSecond), Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Cumulative active time multiplied by queue length.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.disk.merged", DisplayName: "Disk Merged Operations", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Disk reads/writes merged into single physical operations, per second.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.disk.pending_operations", DisplayName: "Disk Pending Operations", Unit: UnitCount, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Instantaneous queue depth of pending disk I/O operations.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},

	// Hostmetrics: filesystem inodes / paging faults / network connections.
	{Name: "system.filesystem.inodes.usage", DisplayName: "Filesystem Inodes Usage", Unit: UnitCount, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Inodes by state (free/used/reserved) per mount point.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.paging.faults", DisplayName: "Page Faults", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Page-fault rate (major/minor) per node.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.network.connections", DisplayName: "Network Connections", Unit: UnitCount, Category: CategoryTraffic, Scopes: []MetricScope{ScopeEntity}, Description: "Instantaneous network connection counts by protocol/state.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
}

func DefaultCatalog() []CanonicalMetric {
	catalog := make([]CanonicalMetric, len(defaultCatalog))
	copy(catalog, defaultCatalog)
	for i := range catalog {
		catalog[i].AppliesTo = cloneEntityKinds(defaultAppliesTo[catalog[i].Name])
		catalog[i].Aggregation = cloneAggregationSpec(catalog[i].Aggregation)
	}
	return catalog
}

func defaultCatalogMetric(name string) (CanonicalMetric, bool) {
	for _, metric := range defaultCatalog {
		if metric.Name == name {
			return metric, true
		}
	}
	return CanonicalMetric{}, false
}

func cloneAggregationSpec(spec *AggregationSpec) *AggregationSpec {
	if spec == nil {
		return nil
	}
	clone := *spec
	clone.Supported = append([]AggregationMode(nil), spec.Supported...)
	return &clone
}

// CatalogByName returns the canonical catalog indexed by semantic metric name.
func CatalogByName() map[string]CanonicalMetric {
	catalog := DefaultCatalog()
	byName := make(map[string]CanonicalMetric, len(catalog))
	for _, metric := range catalog {
		byName[metric.Name] = metric
	}
	return byName
}

var (
	appliesToNode          = []entities.EntityKind{entities.KindNode}
	appliesToNamespace     = []entities.EntityKind{entities.KindNamespace}
	appliesToWorkload      = []entities.EntityKind{entities.KindWorkload}
	appliesToPod           = []entities.EntityKind{entities.KindPod}
	appliesToContainer     = []entities.EntityKind{entities.KindContainer}
	appliesToService       = []entities.EntityKind{entities.KindService}
	appliesToPVCOrPod      = []entities.EntityKind{entities.KindPVC, entities.KindPod}
	appliesToHPA           = []entities.EntityKind{entities.KindHPA}
	appliesToResourceQuota = []entities.EntityKind{entities.KindResourceQuota}
)

var defaultAppliesTo = map[string][]entities.EntityKind{
	"k8s.pod.cpu.usage":                     appliesToPod,
	"k8s.pod.memory.working_set":            appliesToPod,
	"k8s.pod.phase":                         appliesToPod,
	"k8s.pod.status_reason":                 appliesToPod,
	"oksg.pod.health.status":                appliesToPod,
	"k8s.container.cpu.request":             appliesToContainer,
	"k8s.container.cpu.limit":               appliesToContainer,
	"k8s.container.memory.request":          appliesToContainer,
	"k8s.container.memory.limit":            appliesToContainer,
	"k8s.container.cpu.usage":               appliesToContainer,
	"k8s.container.memory.usage":            appliesToContainer,
	"k8s.container.ready":                   appliesToContainer,
	"k8s.container.restarts":                appliesToContainer,
	"oksg.container.health.status":          appliesToContainer,
	"k8s.node.cpu.usage":                    appliesToNode,
	"k8s.node.memory.working_set":           appliesToNode,
	"k8s.node.condition.ready":              appliesToNode,
	"k8s.node.condition.memory_pressure":    appliesToNode,
	"k8s.node.condition.disk_pressure":      appliesToNode,
	"k8s.node.allocatable_cpu":              appliesToNode,
	"k8s.node.allocatable_memory":           appliesToNode,
	"k8s.node.allocatable_pods":             appliesToNode,
	"system.cpu.utilization":                appliesToNode,
	"system.memory.usage":                   appliesToNode,
	"system.disk.io.read":                   appliesToNode,
	"system.disk.io.write":                  appliesToNode,
	"system.network.io.receive":             appliesToNode,
	"system.network.io.transmit":            appliesToNode,
	"system.network.errors":                 appliesToNode,
	"system.network.dropped":                appliesToNode,
	"system.network.packets":                appliesToNode,
	"system.disk.operations":                appliesToNode,
	"system.disk.operation_time":            appliesToNode,
	"system.filesystem.usage":               appliesToNode,
	"system.filesystem.utilization":         appliesToNode,
	"system.cpu.load_average.1m":            appliesToNode,
	"system.cpu.load_average.5m":            appliesToNode,
	"system.cpu.load_average.15m":           appliesToNode,
	"system.paging.usage":                   appliesToNode,
	"system.paging.operations":              appliesToNode,
	"k8s.volume.available":                  appliesToPVCOrPod,
	"k8s.volume.capacity":                   appliesToPVCOrPod,
	"k8s.volume.inodes.free":                appliesToPVCOrPod,
	"k8s.volume.inodes.used":                appliesToPVCOrPod,
	"k8s.namespace.phase":                   appliesToNamespace,
	"k8s.deployment.available":              appliesToWorkload,
	"k8s.deployment.desired":                appliesToWorkload,
	"k8s.replicaset.desired":                appliesToWorkload,
	"k8s.replicaset.available":              appliesToWorkload,
	"k8s.replication_controller.desired":    appliesToWorkload,
	"k8s.replication_controller.available":  appliesToWorkload,
	"k8s.daemonset.desired_scheduled_nodes": appliesToWorkload,
	"k8s.daemonset.current_scheduled_nodes": appliesToWorkload,
	"k8s.daemonset.misscheduled_nodes":      appliesToWorkload,
	"k8s.daemonset.ready_nodes":             appliesToWorkload,
	"k8s.statefulset.desired_pods":          appliesToWorkload,
	"k8s.statefulset.ready_pods":            appliesToWorkload,
	"k8s.statefulset.current_pods":          appliesToWorkload,
	"k8s.statefulset.updated_pods":          appliesToWorkload,
	"k8s.job.active_pods":                   appliesToWorkload,
	"k8s.job.desired_successful_pods":       appliesToWorkload,
	"k8s.job.failed_pods":                   appliesToWorkload,
	"k8s.job.max_parallel_pods":             appliesToWorkload,
	"k8s.job.successful_pods":               appliesToWorkload,
	"k8s.cronjob.active_jobs":               appliesToWorkload,
	"k8s.hpa.max_replicas":                  appliesToHPA,
	"k8s.hpa.min_replicas":                  appliesToHPA,
	"k8s.hpa.current_replicas":              appliesToHPA,
	"k8s.hpa.desired_replicas":              appliesToHPA,
	"k8s.resource_quota.hard_limit":         appliesToResourceQuota,
	"k8s.resource_quota.used":               appliesToResourceQuota,
	"http.server.request.count":             appliesToService,
	"rpc.server.request.count":              appliesToService,
	"servicegraph.request.count":            appliesToService,
	"http.server.request.duration":          appliesToService,
	"rpc.server.duration":                   appliesToService,
	"servicegraph.request.error.count":      appliesToService,
	"servicegraph.request.error.ratio":      appliesToService,

	"k8s.pod.network.io":                      appliesToPod,
	"k8s.pod.network.errors":                  appliesToPod,
	"k8s.pod.filesystem.usage":                appliesToPod,
	"k8s.pod.filesystem.capacity":             appliesToPod,
	"k8s.pod.filesystem.available":            appliesToPod,
	"k8s.node.network.io":                     appliesToNode,
	"k8s.node.network.errors":                 appliesToNode,
	"k8s.node.filesystem.usage":               appliesToNode,
	"k8s.node.filesystem.capacity":            appliesToNode,
	"k8s.node.filesystem.available":           appliesToNode,
	"k8s.container.filesystem.usage":          appliesToContainer,
	"k8s.container.filesystem.capacity":       appliesToContainer,
	"k8s.container.filesystem.available":      appliesToContainer,
	"k8s.container.ephemeral_storage.request": appliesToContainer,
	"k8s.container.ephemeral_storage.limit":   appliesToContainer,
	"k8s.volume.inodes":                       appliesToPVCOrPod,
	"k8s.node.allocatable_ephemeral_storage":  appliesToNode,
	"system.cpu.time":                         appliesToNode,
	"system.disk.io_time":                     appliesToNode,
	"system.disk.weighted_io_time":            appliesToNode,
	"system.disk.merged":                      appliesToNode,
	"system.disk.pending_operations":          appliesToNode,
	"system.filesystem.inodes.usage":          appliesToNode,
	"system.paging.faults":                    appliesToNode,
	"system.network.connections":              appliesToNode,
}

func cloneEntityKinds(kinds []entities.EntityKind) []entities.EntityKind {
	if len(kinds) == 0 {
		return nil
	}
	out := make([]entities.EntityKind, len(kinds))
	copy(out, kinds)
	return out
}
