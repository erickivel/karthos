package servicegraph

// rawMetricNames translates servicegraph connector output into catalog
// canonical names.
var rawMetricNames = map[string]string{
	"traces_service_graph_request_total":        "servicegraph.request.count",
	"traces_service_graph_request_server":       "servicegraph.request.duration",
	"traces_service_graph_request_failed_total": "servicegraph.request.error.count",
	"traces.service.graph.request.total":        "servicegraph.request.count",
	"traces.service.graph.request.server":       "servicegraph.request.duration",
	"traces.service.graph.request_failed_total": "servicegraph.request.error.count",
}
