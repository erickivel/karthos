package metrics

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCounterAggregationSpecsCoverEveryCatalogCounter(t *testing.T) {
	for _, metric := range defaultCatalog {
		mapping, ok := OTLPSemconvMetrics[metric.Name]
		if !ok || mapping.Kind != MetricKindCounter {
			continue
		}

		require.NotNilf(t, metric.Aggregation, "counter %s must declare aggregation metadata", metric.Name)
		require.ElementsMatch(t, counterAggregationModes, metric.Aggregation.Supported)
		require.Contains(t, metric.Aggregation.Supported, metric.Aggregation.Default)
		require.Equal(t, DefaultAggregationWindow, metric.Aggregation.DefaultWindow)
		require.Equal(t, "1m", metric.Aggregation.Window)
		require.NotEmpty(t, metric.Aggregation.BaseUnit)
		require.NotEmpty(t, metric.Aggregation.RateUnit)
	}
}

func TestGaugesAndEnumsDoNotDeclareAggregation(t *testing.T) {
	for _, metric := range defaultCatalog {
		mapping, ok := OTLPSemconvMetrics[metric.Name]
		if !ok || mapping.Kind != MetricKindGauge {
			continue
		}
		require.Nilf(t, metric.Aggregation, "gauge %s must not expose aggregation controls", metric.Name)
	}
}

func TestWindowDerivedMetricsDeclareAggregationInCatalog(t *testing.T) {
	want := map[string]AggregationMode{
		"http.server.request.duration":     AggregationWindowMean,
		"rpc.server.duration":              AggregationWindowMean,
		"servicegraph.request.duration":    AggregationWindowMean,
		"servicegraph.request.body.size":   AggregationWindowMean,
		"servicegraph.response.body.size":  AggregationWindowMean,
		"servicegraph.request.error.ratio": AggregationWindowRatio,
	}

	for _, metric := range defaultCatalog {
		mode, derived := want[metric.Name]
		if !derived {
			continue
		}
		require.NotNilf(t, metric.Aggregation, "%s must declare its derived query behavior", metric.Name)
		require.Equal(t, mode, metric.Aggregation.Default, metric.Name)
		require.Equal(t, DefaultAggregationWindow, metric.Aggregation.DefaultWindow, metric.Name)
		require.Equal(t, "1m", metric.Aggregation.Window, metric.Name)
		require.Empty(t, metric.Aggregation.Supported, metric.Name)
		require.Equal(t, metric.Unit, metric.Aggregation.BaseUnit, metric.Name)
		require.Equal(t, metric.Unit, metric.Aggregation.RateUnit, metric.Name)
		delete(want, metric.Name)
	}
	require.Empty(t, want, "all derived metrics must be present in the catalog")
}

func TestDefaultCatalogDeepCopiesAggregation(t *testing.T) {
	first := CatalogByName()["servicegraph.request.count"]
	require.NotNil(t, first.Aggregation)
	first.Aggregation.Default = AggregationLatest
	first.Aggregation.Supported[0] = AggregationWindowMean

	second := CatalogByName()["servicegraph.request.count"]
	require.Equal(t, AggregationWindowRate, second.Aggregation.Default)
	require.Equal(t, AggregationLatest, second.Aggregation.Supported[0])
}

func TestCounterAggregationDefaultsAndUnits(t *testing.T) {
	catalog := CatalogByName()

	requests := catalog["servicegraph.request.count"].Aggregation
	require.Equal(t, AggregationWindowRate, requests.Default)
	require.Equal(t, time.Minute, requests.DefaultWindow)
	require.Equal(t, UnitCount, requests.BaseUnit)
	require.Equal(t, UnitPerSecond, requests.RateUnit)
	require.Equal(t, "1m", requests.Window)

	restarts := catalog["k8s.container.restarts"].Aggregation
	require.Equal(t, AggregationLatest, restarts.Default)
	require.Equal(t, time.Minute, restarts.DefaultWindow)
	require.Equal(t, "1m", restarts.Window)
	require.Equal(t, UnitCount, restarts.BaseUnit)
	require.Equal(t, UnitPerSecond, restarts.RateUnit)

	cpuTime := catalog["system.cpu.time"].Aggregation
	require.Equal(t, UnitNanoseconds, cpuTime.BaseUnit)
	require.Equal(t, UnitNanosecondsPerSecond, cpuTime.RateUnit)
}

func TestHostCounterAggregationDefaultsAndUnits(t *testing.T) {
	catalog := CatalogByName()

	for _, name := range []string{
		"system.disk.io.read",
		"system.disk.io.write",
		"system.network.io.receive",
		"system.network.io.transmit",
	} {
		spec := catalog[name].Aggregation
		require.NotNilf(t, spec, "%s must expose counter aggregation", name)
		require.Equal(t, AggregationWindowRate, spec.Default, name)
		require.Equal(t, time.Minute, spec.DefaultWindow, name)
		require.Equal(t, UnitBytes, spec.BaseUnit, name)
		require.Equal(t, UnitBytesPerSecond, spec.RateUnit, name)
	}

	paging := catalog["system.paging.operations"].Aggregation
	require.NotNil(t, paging)
	require.Equal(t, AggregationWindowRate, paging.Default)
	require.Equal(t, time.Minute, paging.DefaultWindow)
	require.Equal(t, UnitCount, paging.BaseUnit)
	require.Equal(t, UnitPerSecond, paging.RateUnit)
}

func TestResolveAggregationValidatesModeAndWindow(t *testing.T) {
	catalog := CatalogByName()
	requests := catalog["servicegraph.request.count"]

	resolved, err := ResolveAggregation(requests, "", "")
	require.NoError(t, err)
	require.Equal(t, AggregationWindowRate, resolved.Mode)
	require.Equal(t, time.Minute, resolved.Window)
	require.Equal(t, UnitPerSecond, resolved.Unit)

	resolved, err = ResolveAggregation(requests, string(AggregationIncrease), "90s")
	require.NoError(t, err)
	require.Equal(t, time.Minute+30*time.Second, resolved.Window)
	require.Equal(t, "1m30s", resolved.Window.String())
	require.Equal(t, UnitCount, resolved.Unit)

	restarts := catalog["k8s.container.restarts"]
	resolved, err = ResolveAggregation(restarts, string(AggregationIncrease), "")
	require.NoError(t, err)
	require.Equal(t, AggregationIncrease, resolved.Mode)
	require.Equal(t, time.Minute, resolved.Window)
	require.Equal(t, UnitCount, resolved.Unit)

	_, err = ResolveAggregation(requests, string(AggregationLatest), "1m")
	require.ErrorContains(t, err, "window is not supported")

	_, err = ResolveAggregation(requests, string(AggregationIncrease), "59s")
	require.ErrorContains(t, err, "between 1m0s and 1h0m0s")

	_, err = ResolveAggregation(requests, string(AggregationIncrease), "61m")
	require.ErrorContains(t, err, "between 1m0s and 1h0m0s")

	gauge := catalog["k8s.pod.cpu.usage"]
	_, err = ResolveAggregation(gauge, string(AggregationLatest), "")
	require.ErrorContains(t, err, "aggregation is not supported")
}

func TestCatalogByNameReturnsAggregationMetadata(t *testing.T) {
	catalog := CatalogByName()
	require.Contains(t, catalog, "servicegraph.request.count")
	require.NotNil(t, catalog["servicegraph.request.count"].Aggregation)
}

func TestResolveAggregationAllowsWindowOnlyForDerivedMetrics(t *testing.T) {
	catalog := CatalogByName()

	mean, err := ResolveAggregation(catalog["servicegraph.request.duration"], "", "90s")
	require.NoError(t, err)
	require.Equal(t, AggregationWindowMean, mean.Mode)
	require.Equal(t, 90*time.Second, mean.Window)
	require.Equal(t, UnitNanoseconds, mean.Unit)

	ratio, err := ResolveAggregation(catalog["servicegraph.request.error.ratio"], "", "")
	require.NoError(t, err)
	require.Equal(t, AggregationWindowRatio, ratio.Mode)
	require.Equal(t, time.Minute, ratio.Window)

	_, err = ResolveAggregation(catalog["servicegraph.request.error.ratio"], "increase", "1m")
	require.ErrorContains(t, err, "aggregation is not supported")
}
