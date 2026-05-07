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

// NewDialectRegistryFromSources collects the non-nil Dialects of the given
// logical sources into a single DialectRegistry, preserving order. This is the
// preferred constructor at startup so the dialect set stays in lockstep with
// the source registry.
func NewDialectRegistryFromSources(sources []LogicalSource) *DialectRegistry {
	out := &DialectRegistry{}
	for _, source := range sources {
		if source.Dialect != nil {
			out.dialects = append(out.dialects, source.Dialect)
		}
	}
	return out
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

var KubeletDialect = DialectMap{
	"container.cpu.usage":    "k8s.container.cpu.usage",
	"container.memory.usage": "k8s.container.memory.usage",
}

// KubeClusterDialect translates k8s_cluster receiver wire names (with underscore
// segments per its v0.111+ metadata.yaml) into the dotted catalog canonicals.
// Container ready/restarts already use single-segment names that match the
// catalog directly and need no translation.
var KubeClusterDialect = DialectMap{
	"k8s.container.cpu_request":    "k8s.container.cpu.request",
	"k8s.container.cpu_limit":      "k8s.container.cpu.limit",
	"k8s.container.memory_request": "k8s.container.memory.request",
	"k8s.container.memory_limit":   "k8s.container.memory.limit",
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
}
