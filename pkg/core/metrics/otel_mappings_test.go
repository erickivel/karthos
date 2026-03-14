package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPrometheusToCanonicalMappingsIncludeHTTPAndContainerMetrics(t *testing.T) {
	require.Equal(t, "request_rate_per_second", PrometheusToCanonicalMappings["http_requests_total"])
	require.Equal(t, "request_duration_nanoseconds", PrometheusToCanonicalMappings["http_request_duration_seconds"])
	require.Equal(t, "cpu_usage_millicores", PrometheusToCanonicalMappings["container_cpu_usage_seconds_total"])
	require.Equal(t, "memory_usage_bytes", PrometheusToCanonicalMappings["container_memory_usage_bytes"])
}
