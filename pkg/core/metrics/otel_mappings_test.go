package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestOTLPSemconvMetricsUsesSemconvKeys(t *testing.T) {
	mapping, ok := OTLPSemconvMetrics["k8s.pod.cpu.usage"]
	require.True(t, ok, "expected k8s.pod.cpu.usage to be present in OTLPSemconvMetrics")
	require.Equal(t, OTLPMetricMapping{ConversionFactor: 1000.0, Unit: UnitMillicores, Kind: MetricKindGauge}, mapping)
}

func TestOTLPSemconvMetricsSpanAndServicegraphTargets(t *testing.T) {
	spanDuration, ok := OTLPSemconvMetrics["rpc.server.duration"]
	require.True(t, ok, "expected rpc.server.duration to be present")
	require.Equal(t, UnitNanoseconds, spanDuration.Unit)
	require.Equal(t, MetricKindHistogram, spanDuration.Kind)
	require.Equal(t, 1e6, spanDuration.ConversionFactor)

	servicegraphCount, ok := OTLPSemconvMetrics["servicegraph.request.count"]
	require.True(t, ok, "expected servicegraph.request.count to be present")
	require.Equal(t, UnitPerSecond, servicegraphCount.Unit)
	require.Equal(t, MetricKindCounter, servicegraphCount.Kind)
	require.Equal(t, 1.0, servicegraphCount.ConversionFactor)
}

func TestOTLPSemconvMetricsStoresConditionVariantsAsSemconvStyleNames(t *testing.T) {
	for _, metricName := range []string{
		"k8s.node.condition.ready",
		"k8s.node.condition.memory_pressure",
		"k8s.node.condition.disk_pressure",
	} {
		mapping, ok := OTLPSemconvMetrics[metricName]
		require.True(t, ok, "expected %s to be present", metricName)
		require.Equal(t, UnitCount, mapping.Unit)
		require.Equal(t, MetricKindGauge, mapping.Kind)
	}
}

func TestOTLPSemconvMetricsDoesNotKeepSourceDialectKeys(t *testing.T) {
	for _, sourceName := range []string{
		"http_requests_total",
		"http_request_duration_seconds",
		"container_cpu_usage_seconds_total",
		"traces.span.metrics.calls",
		"traces.service.graph.request.total",
	} {
		_, ok := OTLPSemconvMetrics[sourceName]
		require.False(t, ok, "expected %s to be translated by a dialect instead of stored in OTLPSemconvMetrics", sourceName)
	}
}
