package istio

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTranslatesEverySupportedIstioMetric(t *testing.T) {
	for raw, expected := range map[string]struct {
		canonical string
		factor    float64
	}{
		"istio_requests_total":                {"servicegraph.request.count", 1},
		"istio_request_duration_milliseconds": {"servicegraph.request.duration", 0.001},
		"istio_request_messages_total":        {"servicegraph.grpc.request.message.count", 1},
		"istio_response_messages_total":       {"servicegraph.grpc.response.message.count", 1},
		"istio_tcp_connections_opened_total":  {"servicegraph.tcp.connection.open.count", 1},
		"istio_tcp_connections_closed_total":  {"servicegraph.tcp.connection.close.count", 1},
		"istio_tcp_sent_bytes_total":          {"servicegraph.tcp.bytes.sent", 1},
		"istio_tcp_received_bytes_total":      {"servicegraph.tcp.bytes.received", 1},
	} {
		parsed, ok := Parse(raw, nil)
		require.True(t, ok)
		require.Equal(t, expected.canonical, parsed.CanonicalName)
		require.Equal(t, expected.factor, parsed.ConversionFactor)
	}
}
