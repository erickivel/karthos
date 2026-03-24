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
	// k8scluster
	"k8s.replicaset.desired": {"replica_count", 1.0},
	"k8s.deployment.desired": {"replica_count", 1.0},
	// spanmetrics connector
	"traces.span.metrics.calls":        {"request_rate_per_second", 1.0},
	"traces.span.metrics.duration.sum": {"request_duration_nanoseconds", 1e9}, // seconds → nanoseconds
	// servicegraph connector
	"traces.service.graph.request.total":    {"request_rate_per_second", 1.0},
	"traces.service.graph.request.duration": {"edge_latency_nanoseconds", 1e9}, // seconds → nanoseconds
	// prometheus receiver (forwards Prometheus-style names as-is over OTLP)
	"http_requests_total":               {"request_rate_per_second", 1.0},
	"http_request_duration_seconds":     {"request_duration_nanoseconds", 1.0},
	"container_cpu_usage_seconds_total": {"cpu_usage_millicores", 1.0},
	"container_memory_usage_bytes":      {"memory_usage_bytes", 1.0},
}
