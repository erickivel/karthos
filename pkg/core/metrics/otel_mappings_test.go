package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

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

func TestOTLPToCanonicalMappingsVolumeMetric(t *testing.T) {
	mapping, ok := OTLPToCanonicalMappings["k8s.volume.available"]
	require.True(t, ok, "expected k8s.volume.available to be present")
	require.Equal(t, OTLPMetricMapping{CanonicalName: "volume_available_bytes", ConversionFactor: 1.0}, mapping)
}

func TestOTLPToCanonicalMappingsFilesystemMetric(t *testing.T) {
	mapping, ok := OTLPToCanonicalMappings["system.filesystem.usage"]
	require.True(t, ok, "expected system.filesystem.usage to be present")
	require.Equal(t, OTLPMetricMapping{CanonicalName: "filesystem_usage_bytes", ConversionFactor: 1.0}, mapping)
}

func TestOTLPToCanonicalMappingsLoadAverageMetric(t *testing.T) {
	mapping, ok := OTLPToCanonicalMappings["system.cpu.load_average.1m"]
	require.True(t, ok, "expected system.cpu.load_average.1m to be present")
	require.Equal(t, OTLPMetricMapping{CanonicalName: "cpu_load_average_1m", ConversionFactor: 1.0}, mapping)
}

func TestOTLPToCanonicalMappingsNodeAllocatableCPU(t *testing.T) {
	mapping, ok := OTLPToCanonicalMappings["k8s.node.allocatable_cpu"]
	require.True(t, ok, "expected k8s.node.allocatable_cpu to be present")
	require.Equal(t, 1000.0, mapping.ConversionFactor, "expected cores→millicores conversion")
	require.Equal(t, "node_allocatable_cpu_millicores", mapping.CanonicalName)
}

func TestOTLPToCanonicalMappingsNodeCondition(t *testing.T) {
	mapping, ok := OTLPToCanonicalMappings["k8s.node.condition"]
	require.True(t, ok, "expected k8s.node.condition to be present")
	require.Equal(t, "node_condition_ready", mapping.CanonicalName, "default canonical name for node condition")
}

func TestOTLPToCanonicalMappingsWorkloadState(t *testing.T) {
	tests := []struct {
		otlpName      string
		canonicalName string
	}{
		{"k8s.deployment.available", "deployment_available"},
		{"k8s.daemonset.desired_scheduled_nodes", "daemonset_desired_nodes"},
		{"k8s.daemonset.ready_nodes", "daemonset_ready_nodes"},
		{"k8s.statefulset.desired_pods", "statefulset_desired_pods"},
		{"k8s.statefulset.ready_pods", "statefulset_ready_pods"},
	}
	for _, tt := range tests {
		mapping, ok := OTLPToCanonicalMappings[tt.otlpName]
		require.True(t, ok, "expected %s to be present", tt.otlpName)
		require.Equal(t, tt.canonicalName, mapping.CanonicalName)
	}
}

func TestOTLPToCanonicalMappingsPrometheusDurationUsesNanoseconds(t *testing.T) {
	mapping, ok := OTLPToCanonicalMappings["http_request_duration_seconds"]
	require.True(t, ok, "expected http_request_duration_seconds to be present")
	require.Equal(t, "request_duration_nanoseconds", mapping.CanonicalName)
	require.Equal(t, 1e9, mapping.ConversionFactor)
}

func TestOTLPToCanonicalMappingsSpanMetricsDurationUsesMilliseconds(t *testing.T) {
	mapping, ok := OTLPToCanonicalMappings["traces.span.metrics.duration"]
	require.True(t, ok, "expected traces.span.metrics.duration to be present")
	require.Equal(t, "request_duration_nanoseconds", mapping.CanonicalName)
	require.Equal(t, 1e6, mapping.ConversionFactor)
}

func TestOTLPToCanonicalMappingsServiceGraphMetrics(t *testing.T) {
	serverLatency, ok := OTLPToCanonicalMappings["traces.service.graph.request.server"]
	require.True(t, ok, "expected traces.service.graph.request.server to be present")
	require.Equal(t, "edge_latency_nanoseconds", serverLatency.CanonicalName)
	require.Equal(t, 1e9, serverLatency.ConversionFactor)

	requestTotal, ok := OTLPToCanonicalMappings["traces.service.graph.request.total"]
	require.True(t, ok, "expected traces.service.graph.request.total to be present")
	require.Equal(t, "request_rate_per_second", requestTotal.CanonicalName)

	requestFailed, ok := OTLPToCanonicalMappings["traces.service.graph.request_failed_total"]
	require.True(t, ok, "expected traces.service.graph.request_failed_total to be present")
	require.Equal(t, "error_rate_per_second", requestFailed.CanonicalName)
}

func TestOTLPToCanonicalMappingsUnmappedMetricIsAbsent(t *testing.T) {
	_, ok := OTLPToCanonicalMappings["some.unknown.metric"]
	require.False(t, ok, "expected some.unknown.metric to be absent from OTLPToCanonicalMappings")
}
