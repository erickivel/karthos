package prometheusapp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDialectTranslatesPrometheusHTTPMetrics(t *testing.T) {
	for raw, canonical := range map[string]string{
		"http_requests_total":           "http.server.request.count",
		"http_request_duration_seconds": "http.server.request.duration",
	} {
		got, ok := Dialect.Translate(raw)
		require.True(t, ok)
		require.Equal(t, canonical, got)
	}
}
