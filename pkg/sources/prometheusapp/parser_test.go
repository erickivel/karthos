package prometheusapp

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTranslatesEverySupportedPrometheusMetric(t *testing.T) {
	for raw, expected := range map[string]struct {
		canonical string
		factor    float64
	}{
		"http_requests_total":           {"http.server.request.count", 1},
		"http_request_duration_seconds": {"http.server.request.duration", 1},
		"container_memory_usage_bytes":  {"container.memory.usage", 1},
	} {
		parsed, ok := Parse(raw, nil)
		require.True(t, ok)
		require.Equal(t, expected.canonical, parsed.CanonicalName)
		require.Equal(t, expected.factor, parsed.ConversionFactor)
	}
}

func TestParseRejectsUncollectedContainerCPUCounter(t *testing.T) {
	_, ok := Parse("container_cpu_usage_seconds_total", nil)
	require.False(t, ok)
}
