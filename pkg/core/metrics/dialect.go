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
	bySource map[string]Dialect
}

func NewDialectRegistry(dialects ...Dialect) *DialectRegistry {
	return &DialectRegistry{dialects: dialects}
}

// NewSourceDialectRegistry binds each compiled source ID to its source-owned
// wire-name dialect. Callers obtain these pairs from pkg/sources; metrics does
// not own a parallel source registry.
func NewSourceDialectRegistry(sourceDialects map[string]Dialect) *DialectRegistry {
	out := &DialectRegistry{bySource: make(map[string]Dialect, len(sourceDialects))}
	for sourceID, dialect := range sourceDialects {
		if dialect == nil {
			continue
		}
		out.dialects = append(out.dialects, dialect)
		out.bySource[sourceID] = dialect
	}
	return out
}

// TranslateForSource translates with one explicitly stamped source. The
// source-built registry never performs ordered dialect matching, preventing a
// raw metric from being claimed by an unrelated module.
func (r *DialectRegistry) TranslateForSource(sourceID, metricName string) string {
	if r == nil {
		return metricName
	}
	if r.bySource != nil {
		dialect := r.bySource[sourceID]
		if dialect == nil {
			return metricName
		}
		if translated, ok := dialect.Translate(metricName); ok {
			return translated
		}
		return metricName
	}
	return r.Translate(metricName)
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
// segments per its v0.156 metadata.yaml) into the dotted catalog canonicals.
// Container ready/restarts already use single-segment names that match the
// catalog directly and need no translation.
var KubeClusterDialect = DialectMap{
	"k8s.container.cpu_request":    "k8s.container.cpu.request",
	"k8s.container.cpu_limit":      "k8s.container.cpu.limit",
	"k8s.container.memory_request": "k8s.container.memory.request",
	"k8s.container.memory_limit":   "k8s.container.memory.limit",
}

var ServicegraphDialect = DialectMap{
	"traces_service_graph_request_total":        "servicegraph.request.count",
	"traces_service_graph_request_server":       "servicegraph.request.duration",
	"traces_service_graph_request_failed_total": "servicegraph.request.error.count",
	"traces.service.graph.request.total":        "servicegraph.request.count",
	"traces.service.graph.request.server":       "servicegraph.request.duration",
	"traces.service.graph.request_failed_total": "servicegraph.request.error.count",
}

var IstioDialect = DialectMap{
	"istio_requests_total":                "servicegraph.request.count",
	"istio_request_duration_milliseconds": "servicegraph.request.duration",
}
