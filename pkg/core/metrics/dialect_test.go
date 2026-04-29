package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDialectRegistryFirstMatchWinsAndFallsBackToIdentity(t *testing.T) {
	registry := NewDialectRegistry(
		DialectFromMap(map[string]string{
			"http_requests_total": "custom.http.count",
		}),
		DialectFromMap(map[string]string{
			"http_requests_total": "http.server.request.count",
		}),
	)

	require.Equal(t, "custom.http.count", registry.Translate("http_requests_total"))
	require.Equal(t, "k8s.pod.cpu.usage", registry.Translate("k8s.pod.cpu.usage"))
}

func TestBuiltInDialectsTranslateKnownMetrics(t *testing.T) {
	registry := NewDialectRegistry(PrometheusDialect, SpanmetricsDialect, ServicegraphDialect, IstioDialect)

	require.Equal(t, "http.server.request.count", registry.Translate("http_requests_total"))
	require.Equal(t, "http.server.request.duration", registry.Translate("http_request_duration_seconds"))
	require.Equal(t, "rpc.server.request.count", registry.Translate("traces.span.metrics.calls"))
	require.Equal(t, "rpc.server.duration", registry.Translate("traces.span.metrics.duration"))
	require.Equal(t, "servicegraph.request.count", registry.Translate("traces.service.graph.request.total"))
	require.Equal(t, "servicegraph.request.duration", registry.Translate("traces.service.graph.request.server"))
	require.Equal(t, "servicegraph.request.error.count", registry.Translate("traces.service.graph.request_failed_total"))
	require.Equal(t, "servicegraph.request.count", registry.Translate("istio_requests_total"))
	require.Equal(t, "servicegraph.request.duration", registry.Translate("istio_request_duration_milliseconds"))
	require.Equal(t, "servicegraph.request.body.size", registry.Translate("istio_request_bytes"))
	require.Equal(t, "servicegraph.response.body.size", registry.Translate("istio_response_bytes"))
	require.Equal(t, "servicegraph.grpc.request.message.count", registry.Translate("istio_request_messages_total"))
	require.Equal(t, "servicegraph.grpc.response.message.count", registry.Translate("istio_response_messages_total"))
	require.Equal(t, "servicegraph.tcp.connection.open.count", registry.Translate("istio_tcp_connections_opened_total"))
	require.Equal(t, "servicegraph.tcp.connection.close.count", registry.Translate("istio_tcp_connections_closed_total"))
	require.Equal(t, "istio_tcp_sent_bytes_total", registry.Translate("istio_tcp_sent_bytes_total"))
	require.Equal(t, "istio_tcp_received_bytes_total", registry.Translate("istio_tcp_received_bytes_total"))
}
