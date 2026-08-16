package istio

import "github.com/erickivel/oksg/pkg/core/metrics"

// Dialect translates Istio mesh telemetry names into catalog canonical names.
var Dialect = metrics.DialectMap{
	"istio_requests_total":                "servicegraph.request.count",
	"istio_request_duration_milliseconds": "servicegraph.request.duration",
	"istio_request_messages_total":        "servicegraph.grpc.request.message.count",
	"istio_response_messages_total":       "servicegraph.grpc.response.message.count",
	"istio_tcp_connections_opened_total":  "servicegraph.tcp.connection.open.count",
	"istio_tcp_connections_closed_total":  "servicegraph.tcp.connection.close.count",
	"istio_tcp_sent_bytes_total":          "servicegraph.tcp.bytes.sent",
	"istio_tcp_received_bytes_total":      "servicegraph.tcp.bytes.received",
}
