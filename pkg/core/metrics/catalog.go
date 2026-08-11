package metrics

import "github.com/erickivel/oksg/pkg/core/entities"

var defaultCatalog = []CanonicalMetric{
	{Name: "k8s.pod.cpu.usage", DisplayName: "Pod CPU Usage", Unit: UnitMillicores, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed pod CPU usage.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.pod.memory.working_set", DisplayName: "Pod Memory Working Set", Unit: UnitBytes, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed pod memory working set.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.node.cpu.usage", DisplayName: "Node CPU Usage", Unit: UnitMillicores, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed node CPU usage from kubeletstats.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.node.memory.working_set", DisplayName: "Node Memory Working Set", Unit: UnitBytes, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed node memory usage from kubeletstats.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.container.cpu.usage", DisplayName: "Container CPU Usage", Unit: UnitMillicores, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed container CPU usage.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.container.memory.usage", DisplayName: "Container Memory Usage", Unit: UnitBytes, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed container memory usage.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.volume.available", DisplayName: "Volume Available Bytes", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Available bytes on a pod volume.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.volume.capacity", DisplayName: "Volume Capacity", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Total capacity bytes on a pod volume.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.volume.inodes.free", DisplayName: "Volume Inodes Free", Unit: UnitCount, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Free inodes on a pod volume.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.volume.inodes.used", DisplayName: "Volume Inodes Used", Unit: UnitCount, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Used inodes on a pod volume.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "k8s.volume.inodes", DisplayName: "Volume Inodes Total", Unit: UnitCount, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Total inodes on a pod volume.", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	{Name: "system.cpu.utilization", DisplayName: "Host CPU Utilization", Unit: UnitPercent, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed host CPU utilization.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.memory.usage", DisplayName: "Host Memory Usage", Unit: UnitBytes, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed host memory usage.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.disk.io.read", DisplayName: "Disk Read Throughput", Unit: UnitBytesPerSecond, Aggregation: windowRateCounterAggregation(UnitBytes, UnitBytesPerSecond), Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed disk read throughput.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.disk.io.write", DisplayName: "Disk Write Throughput", Unit: UnitBytesPerSecond, Aggregation: windowRateCounterAggregation(UnitBytes, UnitBytesPerSecond), Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Observed disk write throughput.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.disk.operations", DisplayName: "Disk Operations", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Disk I/O operation rate.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.filesystem.usage", DisplayName: "Filesystem Usage", Unit: UnitBytes, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Filesystem usage by mount point.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.filesystem.utilization", DisplayName: "Filesystem Utilization", Unit: UnitPercent, Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Filesystem utilization by mount point.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.network.io.receive", DisplayName: "Network Receive Throughput", Unit: UnitBytesPerSecond, Aggregation: windowRateCounterAggregation(UnitBytes, UnitBytesPerSecond), Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Network receive throughput.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.network.io.transmit", DisplayName: "Network Transmit Throughput", Unit: UnitBytesPerSecond, Aggregation: windowRateCounterAggregation(UnitBytes, UnitBytesPerSecond), Category: CategoryResource, Scopes: []MetricScope{ScopeEntity}, Description: "Network transmit throughput.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "system.network.errors", DisplayName: "Network Errors", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryError, Scopes: []MetricScope{ScopeEntity}, Description: "Network interface error rate.", Sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
	{Name: "k8s.container.cpu.request", DisplayName: "Container CPU Request", Unit: UnitMillicores, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Requested CPU for a container.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.container.cpu.limit", DisplayName: "Container CPU Limit", Unit: UnitMillicores, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "CPU limit for a container.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.container.memory.request", DisplayName: "Container Memory Request", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Requested memory for a container.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.container.memory.limit", DisplayName: "Container Memory Limit", Unit: UnitBytes, Category: CategoryCapacity, Scopes: []MetricScope{ScopeEntity}, Description: "Memory limit for a container.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.container.ready", DisplayName: "Container Ready", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Container Ready condition.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.container.restarts", DisplayName: "Container Restarts", Unit: UnitCount, Aggregation: latestCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Cumulative container restart count.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.pod.phase", DisplayName: "Pod Phase", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Integer-encoded pod lifecycle phase.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.pod.status_reason", DisplayName: "Pod Status Reason", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Integer-encoded pod failure reason.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.node.condition.ready", DisplayName: "Node Ready", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Node Ready condition.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.node.condition.memory_pressure", DisplayName: "Node Memory Pressure", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Node MemoryPressure condition.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "k8s.node.condition.disk_pressure", DisplayName: "Node Disk Pressure", Unit: UnitCount, Category: CategoryHealth, Scopes: []MetricScope{ScopeEntity}, Description: "Node DiskPressure condition.", Sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
	{Name: "http.server.request.count", DisplayName: "HTTP Request Rate", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEntity}, Description: "HTTP request rate from declared application metrics.", Sources: []SourceRef{{ID: SourcePrometheusApp, Priority: 1}}},
	{Name: "http.server.request.duration", DisplayName: "HTTP Request Duration", Unit: UnitNanoseconds, Aggregation: windowDerivedAggregation(AggregationWindowMean, UnitNanoseconds), Category: CategoryLatency, Scopes: []MetricScope{ScopeEntity}, Description: "HTTP request duration from declared application metrics.", Sources: []SourceRef{{ID: SourcePrometheusApp, Priority: 1}}},
	{Name: "servicegraph.request.count", DisplayName: "Service Graph Request Rate", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEntity, ScopeEdge}, Description: "Request rate for a service or edge.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}, {ID: SourceTracesServicegraph, Priority: 2}}},
	{Name: "servicegraph.request.duration", DisplayName: "Service Graph Request Duration", Unit: UnitNanoseconds, Aggregation: windowDerivedAggregation(AggregationWindowMean, UnitNanoseconds), Category: CategoryLatency, Scopes: []MetricScope{ScopeEdge}, Description: "Request latency between graph edges.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}, {ID: SourceTracesServicegraph, Priority: 2}}},
	{Name: "servicegraph.request.error.count", DisplayName: "Service Graph Error Rate", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryError, Scopes: []MetricScope{ScopeEntity, ScopeEdge}, Description: "Error rate for a service or edge.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}, {ID: SourceTracesServicegraph, Priority: 2}}},
	{Name: "servicegraph.grpc.request.message.count", DisplayName: "Service Graph gRPC Request Message Rate", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEdge}, Description: "gRPC request messages observed by the Istio mesh.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}}},
	{Name: "servicegraph.grpc.response.message.count", DisplayName: "Service Graph gRPC Response Message Rate", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEdge}, Description: "gRPC response messages observed by the Istio mesh.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}}},
	{Name: "servicegraph.tcp.connection.open.count", DisplayName: "Service Graph TCP Open Connection Rate", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEdge}, Description: "TCP connections opened between services in the Istio mesh.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}}},
	{Name: "servicegraph.tcp.connection.close.count", DisplayName: "Service Graph TCP Close Connection Rate", Unit: UnitPerSecond, Aggregation: windowRateCounterAggregation(UnitCount, UnitPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEdge}, Description: "TCP connections closed between services in the Istio mesh.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}}},
	{Name: "servicegraph.tcp.bytes.sent", DisplayName: "Service Graph TCP Sent Throughput", Unit: UnitBytesPerSecond, Aggregation: windowRateCounterAggregation(UnitBytes, UnitBytesPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEdge}, Description: "TCP bytes sent between services in the Istio mesh.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}}},
	{Name: "servicegraph.tcp.bytes.received", DisplayName: "Service Graph TCP Received Throughput", Unit: UnitBytesPerSecond, Aggregation: windowRateCounterAggregation(UnitBytes, UnitBytesPerSecond), Category: CategoryTraffic, Scopes: []MetricScope{ScopeEdge}, Description: "TCP bytes received between services in the Istio mesh.", Sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}}},
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
	appliesToNode      = []entities.EntityKind{entities.KindNode}
	appliesToPod       = []entities.EntityKind{entities.KindPod}
	appliesToContainer = []entities.EntityKind{entities.KindContainer}
	appliesToService   = []entities.EntityKind{entities.KindService}
	appliesToPVCOrPod  = []entities.EntityKind{entities.KindPVC, entities.KindPod}
)

var defaultAppliesTo = map[string][]entities.EntityKind{
	"k8s.pod.cpu.usage":                  appliesToPod,
	"k8s.pod.memory.working_set":         appliesToPod,
	"k8s.pod.phase":                      appliesToPod,
	"k8s.pod.status_reason":              appliesToPod,
	"k8s.container.cpu.request":          appliesToContainer,
	"k8s.container.cpu.limit":            appliesToContainer,
	"k8s.container.memory.request":       appliesToContainer,
	"k8s.container.memory.limit":         appliesToContainer,
	"k8s.container.cpu.usage":            appliesToContainer,
	"k8s.container.memory.usage":         appliesToContainer,
	"k8s.container.ready":                appliesToContainer,
	"k8s.container.restarts":             appliesToContainer,
	"k8s.node.cpu.usage":                 appliesToNode,
	"k8s.node.memory.working_set":        appliesToNode,
	"k8s.node.condition.ready":           appliesToNode,
	"k8s.node.condition.memory_pressure": appliesToNode,
	"k8s.node.condition.disk_pressure":   appliesToNode,
	"system.cpu.utilization":             appliesToNode,
	"system.memory.usage":                appliesToNode,
	"system.disk.io.read":                appliesToNode,
	"system.disk.io.write":               appliesToNode,
	"system.network.io.receive":          appliesToNode,
	"system.network.io.transmit":         appliesToNode,
	"system.network.errors":              appliesToNode,
	"system.disk.operations":             appliesToNode,
	"system.filesystem.usage":            appliesToNode,
	"system.filesystem.utilization":      appliesToNode,
	"k8s.volume.available":               appliesToPVCOrPod,
	"k8s.volume.capacity":                appliesToPVCOrPod,
	"k8s.volume.inodes.free":             appliesToPVCOrPod,
	"k8s.volume.inodes.used":             appliesToPVCOrPod,
	"k8s.volume.inodes":                  appliesToPVCOrPod,
	"http.server.request.count":          appliesToService,
	"servicegraph.request.count":         appliesToService,
	"http.server.request.duration":       appliesToService,
	"servicegraph.request.error.count":   appliesToService,
}

func cloneEntityKinds(kinds []entities.EntityKind) []entities.EntityKind {
	if len(kinds) == 0 {
		return nil
	}
	out := make([]entities.EntityKind, len(kinds))
	copy(out, kinds)
	return out
}
