package metrics

// OTLPMetricMapping describes a canonical mapping with optional unit conversion.
type OTLPMetricMapping struct {
	CanonicalName    string
	ConversionFactor float64 // multiply raw value by this; 1.0 = no conversion
}

// OTLPToCanonicalMappings maps OTLP metric names to canonical metric names and unit conversion factors.
var OTLPToCanonicalMappings = map[string]OTLPMetricMapping{
	// kubeletstats
	"k8s.pod.cpu.usage":            {"cpu_usage_millicores", 1000.0}, // cores → millicores
	"k8s.pod.memory.working_set":   {"memory_usage_bytes", 1.0},
	"k8s.node.cpu.usage":           {"cpu_usage_millicores", 1000.0},
	"k8s.node.memory.working_set":  {"memory_usage_bytes", 1.0},
	"k8s.container.cpu.request":    {"cpu_request_millicores", 1000.0},
	"k8s.container.cpu.limit":      {"cpu_limit_millicores", 1000.0},
	"k8s.container.memory.request": {"memory_request_bytes", 1.0},
	"k8s.container.memory.limit":   {"memory_limit_bytes", 1.0},
	// hostmetrics
	"system.cpu.utilization": {"cpu_usage_millicores", 1000.0}, // ratio → millicores approx
	"system.memory.usage":    {"memory_usage_bytes", 1.0},
	"system.disk.io":         {"disk_read_bps", 1.0}, // direction resolved at normalization time
	"system.network.io":      {"net_rx_bps", 1.0},    // direction resolved at normalization time
	// kubeletstats volume
	"k8s.volume.available":   {"volume_available_bytes", 1.0},
	"k8s.volume.capacity":    {"volume_capacity_bytes", 1.0},
	"k8s.volume.inodes.free": {"volume_inodes_free", 1.0},
	"k8s.volume.inodes.used": {"volume_inodes_used", 1.0},
	// hostmetrics filesystem
	"system.filesystem.usage":       {"filesystem_usage_bytes", 1.0},
	"system.filesystem.utilization": {"filesystem_utilization", 1.0},
	// hostmetrics load
	"system.cpu.load_average.1m":  {"cpu_load_average_1m", 1.0},
	"system.cpu.load_average.5m":  {"cpu_load_average_5m", 1.0},
	"system.cpu.load_average.15m": {"cpu_load_average_15m", 1.0},
	// hostmetrics paging
	"system.paging.usage":      {"paging_usage_bytes", 1.0},
	"system.paging.operations": {"paging_operations_per_second", 1.0},
	// k8scluster
	"k8s.replicaset.desired":   {"replica_count", 1.0},
	"k8s.deployment.desired":   {"replica_count", 1.0},
	"k8s.deployment.available": {"deployment_available", 1.0},
	// k8scluster state
	"k8s.pod.phase":               {"pod_phase", 1.0},
	"k8s.namespace.phase":         {"namespace_phase", 1.0},
	"k8s.node.condition":          {"node_condition_ready", 1.0},               // condition type resolved at normalization time
	"k8s.node.allocatable_cpu":    {"node_allocatable_cpu_millicores", 1000.0}, // cores → millicores
	"k8s.node.allocatable_memory": {"node_allocatable_memory_bytes", 1.0},
	// k8scluster workload state
	"k8s.daemonset.desired_scheduled_nodes": {"daemonset_desired_nodes", 1.0},
	"k8s.daemonset.ready_nodes":             {"daemonset_ready_nodes", 1.0},
	"k8s.statefulset.desired_pods":          {"statefulset_desired_pods", 1.0},
	"k8s.statefulset.ready_pods":            {"statefulset_ready_pods", 1.0},
	// spanmetrics connector
	"traces.span.metrics.calls":    {"request_rate_per_second", 1.0},
	"traces.span.metrics.duration": {"request_duration_nanoseconds", 1e6}, // milliseconds → nanoseconds
	// keep legacy mapping for any pre-existing serialized samples/tests
	"traces.span.metrics.duration.sum": {"request_duration_nanoseconds", 1e6}, // milliseconds → nanoseconds
	// servicegraph connector
	"traces.service.graph.request.total":        {"request_rate_per_second", 1.0},
	"traces.service.graph.request.server":       {"edge_latency_nanoseconds", 1e9}, // seconds → nanoseconds
	"traces.service.graph.request_failed_total": {"error_rate_per_second", 1.0},
	// prometheus receiver (forwards Prometheus-style names as-is over OTLP)
	"http_requests_total":               {"request_rate_per_second", 1.0},
	"http_request_duration_seconds":     {"request_duration_nanoseconds", 1e9},
	"container_cpu_usage_seconds_total": {"cpu_usage_millicores", 1.0},
	"container_memory_usage_bytes":      {"memory_usage_bytes", 1.0},
}
