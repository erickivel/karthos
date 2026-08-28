package istio

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTranslatesIstioProtocolMetrics(t *testing.T) {
	for raw, canonical := range map[string]string{
		"istio_requests_total":                "servicegraph.request.count",
		"istio_request_duration_milliseconds": "servicegraph.request.duration",
		"istio_tcp_sent_bytes_total":          "servicegraph.tcp.bytes.sent",
	} {
		parsed, ok := Parse(raw, nil)
		require.True(t, ok)
		require.Equal(t, canonical, parsed.CanonicalName)
	}
}
