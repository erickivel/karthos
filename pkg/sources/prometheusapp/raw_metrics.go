package prometheusapp

// rawMetricNames translates supported Prometheus scrape names into catalog
// canonical names.
var rawMetricNames = map[string]string{
	"http_requests_total":           "http.server.request.count",
	"http_request_duration_seconds": "http.server.request.duration",
	"container_memory_usage_bytes":  "container.memory.usage",
}
