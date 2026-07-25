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
	require.Equal(t, UnitCount, servicegraphCount.Unit)
	require.Equal(t, MetricKindCounter, servicegraphCount.Kind)
	require.Equal(t, 1.0, servicegraphCount.ConversionFactor)

	requestBytes, ok := OTLPSemconvMetrics["servicegraph.request.body.size"]
	require.True(t, ok, "expected servicegraph.request.body.size to be present")
	require.Equal(t, UnitBytes, requestBytes.Unit)
	require.Equal(t, MetricKindHistogram, requestBytes.Kind)

	tcpConnections, ok := OTLPSemconvMetrics["servicegraph.tcp.connection.open.count"]
	require.True(t, ok, "expected servicegraph.tcp.connection.open.count to be present")
	require.Equal(t, UnitCount, tcpConnections.Unit)
	require.Equal(t, MetricKindCounter, tcpConnections.Kind)
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

func TestOTLPSemconvMetricsResourceQuotaUsesResourceUnit(t *testing.T) {
	for _, metricName := range []string{
		"k8s.resource_quota.hard_limit",
		"k8s.resource_quota.used",
	} {
		mapping, ok := OTLPSemconvMetrics[metricName]
		require.True(t, ok, "expected %s to be present", metricName)
		require.Equal(t, UnitResource, mapping.Unit)
		require.Equal(t, MetricKindGauge, mapping.Kind)
	}
}

func TestOTLPSemconvMetricsRemovesGenericContainerStorage(t *testing.T) {
	require.NotContains(t, OTLPSemconvMetrics, "k8s.container.storage.request")
	require.NotContains(t, OTLPSemconvMetrics, "k8s.container.storage.limit")
	require.Contains(t, OTLPSemconvMetrics, "k8s.container.ephemeral_storage.request")
	require.Contains(t, OTLPSemconvMetrics, "k8s.container.ephemeral_storage.limit")
}

func TestOTLPSemconvMetricsHostByteAndPagingSumsAreCounters(t *testing.T) {
	for _, metricName := range []string{
		"system.disk.io",
		"system.disk.io.read",
		"system.disk.io.write",
		"system.network.io",
		"system.network.io.receive",
		"system.network.io.transmit",
		"system.paging.operations",
	} {
		mapping, ok := OTLPSemconvMetrics[metricName]
		require.True(t, ok, "expected %s to be present", metricName)
		require.Equal(t, MetricKindCounter, mapping.Kind, "%s must preserve its cumulative monotonic sum contract", metricName)
	}
}

func TestOTLPSemconvMetricCountersStoreOnlyBaseUnits(t *testing.T) {
	for metricName, mapping := range OTLPSemconvMetrics {
		if mapping.Kind != MetricKindCounter {
			continue
		}

		require.Containsf(t, []Unit{UnitCount, UnitBytes, UnitNanoseconds}, mapping.Unit,
			"%s must store the raw cumulative counter in a base unit", metricName)
	}
}

func TestCatalogCounterRatesAreDerivedFromStoredBaseUnits(t *testing.T) {
	catalog := CatalogByName()
	for metricName, mapping := range OTLPSemconvMetrics {
		if mapping.Kind != MetricKindCounter {
			continue
		}

		metric, ok := catalog[metricName]
		if !ok {
			continue
		}
		require.NotNilf(t, metric.Aggregation, "%s must declare query-time aggregation", metricName)
		require.Equalf(t, mapping.Unit, metric.Aggregation.BaseUnit,
			"%s storage unit must equal the catalog base unit", metricName)
		if metric.Aggregation.Default == AggregationWindowRate {
			require.NotEqualf(t, mapping.Unit, metric.Aggregation.RateUnit,
				"%s rate unit must be query-derived rather than stored", metricName)
		}
	}
}

func TestOTLPSemconvMetricsDoesNotKeepSourceDialectKeys(t *testing.T) {
	for _, sourceName := range []string{
		"http_requests_total",
		"http_request_duration_seconds",
		"container_cpu_usage_seconds_total",
		"traces.span.metrics.calls",
		"traces_service_graph_request_total",
		"istio_requests_total",
		"istio_request_duration_milliseconds",
	} {
		_, ok := OTLPSemconvMetrics[sourceName]
		require.False(t, ok, "expected %s to be translated by a dialect instead of stored in OTLPSemconvMetrics", sourceName)
	}
}
