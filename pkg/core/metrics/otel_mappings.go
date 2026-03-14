package metrics

var PrometheusToCanonicalMappings = map[string]string{
	"http_requests_total":               "request_rate_per_second",
	"http_request_duration_seconds":     "request_duration_nanoseconds",
	"container_cpu_usage_seconds_total": "cpu_usage_millicores",
	"container_memory_usage_bytes":      "memory_usage_bytes",
}
