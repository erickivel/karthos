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

func TestOTLPSemconvMetricsServicegraphTargets(t *testing.T) {
	servicegraphCount, ok := OTLPSemconvMetrics["servicegraph.request.count"]
	require.True(t, ok, "expected servicegraph.request.count to be present")
	require.Equal(t, UnitCount, servicegraphCount.Unit)
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

func TestOTLPSemconvMetricsHostByteAndPagingSumsAreCounters(t *testing.T) {
	for _, metricName := range []string{
		"system.disk.io.read",
		"system.disk.io.write",
		"system.network.io.receive",
		"system.network.io.transmit",
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
