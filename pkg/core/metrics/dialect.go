package metrics

type Dialect interface {
	Translate(metricName string) (string, bool)
}

type DialectMap map[string]string

func (d DialectMap) Translate(metricName string) (string, bool) {
	translated, ok := d[metricName]
	return translated, ok
}

func DialectFromMap(m map[string]string) Dialect {
	return DialectMap(m)
}

type DialectRegistry struct {
	dialects []Dialect
}

func NewDialectRegistry(dialects ...Dialect) *DialectRegistry {
	return &DialectRegistry{dialects: dialects}
}

func (r *DialectRegistry) Translate(metricName string) string {
	if r == nil {
		return metricName
	}
	for _, dialect := range r.dialects {
		if translated, ok := dialect.Translate(metricName); ok {
			return translated
		}
	}
	return metricName
}

var PrometheusDialect = DialectMap{
	"http_requests_total":               "http.server.request.count",
	"http_request_duration_seconds":     "http.server.request.duration",
	"container_cpu_usage_seconds_total": "k8s.container.cpu.usage",
	"container_memory_usage_bytes":      "k8s.container.memory.usage",
}

var SpanmetricsDialect = DialectMap{
	"traces.span.metrics.calls":    "rpc.server.request.count",
	"traces.span.metrics.duration": "rpc.server.duration",
}

var ServicegraphDialect = DialectMap{
	"traces.service.graph.request.total":        "servicegraph.request.count",
	"traces.service.graph.request.server":       "servicegraph.request.duration",
	"traces.service.graph.request_failed_total": "servicegraph.request.error.count",
}

var IstioDialect = DialectMap{
	"istio_requests_total":                "servicegraph.request.count",
	"istio_request_duration_milliseconds": "servicegraph.request.duration",
	"istio_request_bytes":                 "servicegraph.request.body.size",
	"istio_response_bytes":                "servicegraph.response.body.size",
	"istio_request_messages_total":        "servicegraph.grpc.request.message.count",
	"istio_response_messages_total":       "servicegraph.grpc.response.message.count",
	"istio_tcp_connections_opened_total":  "servicegraph.tcp.connection.open.count",
	"istio_tcp_connections_closed_total":  "servicegraph.tcp.connection.close.count",
	"istio_tcp_sent_bytes_total":          "k8s.pod.network.io.transmit",
	"istio_tcp_received_bytes_total":      "k8s.pod.network.io.receive",
}
