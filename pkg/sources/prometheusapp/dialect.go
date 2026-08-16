package prometheusapp

import "github.com/erickivel/oksg/pkg/core/metrics"

// Dialect translates supported Prometheus scrape names into catalog canonical names.
var Dialect = metrics.DialectMap{
	"http_requests_total":               "http.server.request.count",
	"http_request_duration_seconds":     "http.server.request.duration",
	"container_cpu_usage_seconds_total": "k8s.container.cpu.usage",
	"container_memory_usage_bytes":      "k8s.container.memory.usage",
}
