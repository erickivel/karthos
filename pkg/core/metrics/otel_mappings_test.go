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

func TestOTLPToCanonicalMappingsKubeletstatsMetric(t *testing.T) {
	mapping, ok := OTLPToCanonicalMappings["k8s.pod.cpu.usage"]
	require.True(t, ok, "expected k8s.pod.cpu.usage to be present in OTLPToCanonicalMappings")
	require.Equal(t, OTLPMetricMapping{CanonicalName: "cpu_usage_millicores", ConversionFactor: 1000.0}, mapping)
}

func TestOTLPToCanonicalMappingsHostmetricsMetric(t *testing.T) {
	mapping, ok := OTLPToCanonicalMappings["system.memory.usage"]
	require.True(t, ok, "expected system.memory.usage to be present in OTLPToCanonicalMappings")
	require.Equal(t, OTLPMetricMapping{CanonicalName: "memory_usage_bytes", ConversionFactor: 1.0}, mapping)
}

func TestOTLPToCanonicalMappingsUnmappedMetricIsAbsent(t *testing.T) {
	_, ok := OTLPToCanonicalMappings["some.unknown.metric"]
	require.False(t, ok, "expected some.unknown.metric to be absent from OTLPToCanonicalMappings")
}
