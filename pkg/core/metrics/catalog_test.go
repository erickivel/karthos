package metrics

import (
	"testing"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/stretchr/testify/require"
)

func TestDefaultCatalogIncludesRequiredMetrics(t *testing.T) {
	type metricExpectation struct {
		unit     Unit
		category MetricCategory
		sources  []SourceRef
	}

	required := map[string]metricExpectation{
		"k8s.pod.cpu.usage":                  {unit: UnitMillicores, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.pod.memory.working_set":         {unit: UnitBytes, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.node.cpu.usage":                 {unit: UnitMillicores, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.node.memory.working_set":        {unit: UnitBytes, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"system.cpu.utilization":             {unit: UnitPercent, category: CategoryResource, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
		"system.memory.usage":                {unit: UnitBytes, category: CategoryResource, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
		"system.disk.io.read":                {},
		"system.disk.io.write":               {},
		"system.disk.operations":             {},
		"system.filesystem.usage":            {},
		"system.filesystem.utilization":      {},
		"system.network.io.receive":          {},
		"system.network.io.transmit":         {},
		"system.network.errors":              {unit: UnitPerSecond, category: CategoryError, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
		"servicegraph.request.count":         {},
		"http.server.request.duration":       {},
		"servicegraph.request.duration":      {},
		"servicegraph.request.error.count":   {},
		"k8s.container.cpu.request":          {},
		"k8s.container.cpu.limit":            {},
		"k8s.container.memory.request":       {},
		"k8s.container.memory.limit":         {},
		"k8s.container.cpu.usage":            {unit: UnitMillicores, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.container.memory.usage":         {unit: UnitBytes, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.container.ready":                {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.container.restarts":             {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.volume.available":               {},
		"k8s.volume.capacity":                {},
		"k8s.volume.inodes.free":             {},
		"k8s.volume.inodes.used":             {},
		"k8s.pod.phase":                      {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.pod.status_reason":              {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.node.condition.ready":           {},
		"k8s.node.condition.memory_pressure": {},
		"k8s.node.condition.disk_pressure":   {},
		"k8s.volume.inodes":                  {unit: UnitCount, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	}

	for _, metric := range DefaultCatalog() {
		expected, ok := required[metric.Name]
		if !ok {
			continue
		}
		if expected.unit != "" {
			require.Equalf(t, expected.unit, metric.Unit, "metric %s unit", metric.Name)
		}
		if expected.category != "" {
			require.Equalf(t, expected.category, metric.Category, "metric %s category", metric.Name)
		}
		if expected.sources != nil {
			require.Equalf(t, expected.sources, metric.Sources, "metric %s sources", metric.Name)
		}
		delete(required, metric.Name)
	}

	require.Empty(t, required)
}

func TestDefaultCatalogExcludesUnsupportedMetrics(t *testing.T) {
	unsupported := map[string]struct{}{
		"rpc.server.request.count":                {},
		"rpc.server.duration":                     {},
		"oksg.pod.health.status":                  {},
		"oksg.container.health.status":            {},
		"k8s.deployment.desired":                  {},
		"k8s.node.allocatable_cpu":                {},
		"k8s.container.ephemeral_storage.request": {},
		"system.cpu.load_average.1m":              {},
		"system.network.dropped":                  {},
		"k8s.pod.network.io":                      {},
		"k8s.pod.filesystem.usage":                {},
		"k8s.node.cpu.capacity":                   {},
		"k8s.node.cpu.cores":                      {},
		"k8s.node.cpu.frequency":                  {},
		"k8s.node.memory.capacity":                {},
		"k8s.node.disk.capacity":                  {},
		"k8s.node.network.bandwidth.receive":      {},
		"k8s.node.network.bandwidth.transmit":     {},
		"k8s.pod.network.io.receive":              {},
		"k8s.pod.network.io.transmit":             {},
	}

	for _, metric := range DefaultCatalog() {
		_, exists := unsupported[metric.Name]
		require.Falsef(t, exists, "unsupported metric should not be cataloged: %s", metric.Name)
	}
}

func TestDefaultCatalogMetricsHaveIngestOrQuerySupport(t *testing.T) {
	for _, metric := range DefaultCatalog() {
		if _, ok := catalogSourcelessExceptions[metric.Name]; ok {
			continue
		}
		_, mapped := OTLPSemconvMetrics[metric.Name]
		require.Truef(t, mapped, "catalog metric %s has no OTLP mapping or query-derived exception", metric.Name)
	}
}

func TestDefaultCatalogMetricNamesAreUnique(t *testing.T) {
	seen := make(map[string]struct{})

	for _, metric := range DefaultCatalog() {
		_, exists := seen[metric.Name]
		require.Falsef(t, exists, "duplicate metric in catalog: %s", metric.Name)
		seen[metric.Name] = struct{}{}
	}
}

func TestDefaultCatalogEntriesHaveDisplayNames(t *testing.T) {
	for _, metric := range DefaultCatalog() {
		require.NotEmptyf(t, metric.DisplayName, "metric %s is missing DisplayName", metric.Name)
	}
}

func TestDefaultCatalogEntityMetricsDeclareApplicableKinds(t *testing.T) {
	for _, metric := range DefaultCatalog() {
		if hasScope(metric, ScopeEntity) {
			require.NotEmptyf(t, metric.AppliesTo, "entity metric %s declares no AppliesTo kinds", metric.Name)
			continue
		}
		require.Emptyf(t, metric.AppliesTo, "edge-only metric %s must not declare entity AppliesTo kinds", metric.Name)
	}
}

func TestDefaultCatalogApplicabilityTargetsExpectedKinds(t *testing.T) {
	catalog := catalogByName(DefaultCatalog())

	require.Equal(t, []entities.EntityKind{entities.KindContainer}, catalog["k8s.container.cpu.usage"].AppliesTo)
	require.Equal(t, []entities.EntityKind{entities.KindContainer}, catalog["k8s.container.ready"].AppliesTo)
	require.Equal(t, []entities.EntityKind{entities.KindPod}, catalog["k8s.pod.cpu.usage"].AppliesTo)
	require.Equal(t, []entities.EntityKind{entities.KindNode}, catalog["k8s.node.cpu.usage"].AppliesTo)
	require.Equal(t, []entities.EntityKind{entities.KindPVC, entities.KindPod}, catalog["k8s.volume.inodes.used"].AppliesTo)
	require.Equal(t, []entities.EntityKind{entities.KindService}, catalog["servicegraph.request.count"].AppliesTo)
	require.Empty(t, catalog["servicegraph.request.duration"].AppliesTo)
}

func TestDefaultCatalogRemovesDeferredMetricFamilies(t *testing.T) {
	catalog := catalogByName(DefaultCatalog())
	require.NotContains(t, catalog, "k8s.container.storage.request")
	require.NotContains(t, catalog, "k8s.container.storage.limit")
	require.NotContains(t, catalog, "k8s.container.ephemeral_storage.request")
	require.NotContains(t, catalog, "oksg.pod.health.status")
}

// Query-derived metrics — composed at query time from other catalog metrics
// (see pkg/adapters/storage/victoriametrics/metric_store.go buildPromQL switch
// arms, and the derivedHealthStatus helper for cross-entity composition).
// They never arrive as OTLP samples, so they intentionally have no source.
//
// When adding a new derived metric here you MUST also add a buildPromQL case
// (or extend derivedHealthStatus) before the metric is exposed via the
// catalog test's `required` map — otherwise queries return empty silently.
var catalogSourcelessExceptions = map[string]struct{}{
	"oksg.pod.health.status":       {},
	"oksg.container.health.status": {},
}

func TestDefaultCatalogEntriesDeclareSources(t *testing.T) {
	for _, metric := range DefaultCatalog() {
		if _, ok := catalogSourcelessExceptions[metric.Name]; ok {
			continue
		}
		require.NotEmptyf(t, metric.Sources, "metric %s declares no Sources", metric.Name)
	}
}

func TestDefaultCatalogSourceRefsTargetKnownSources(t *testing.T) {
	known := map[string]struct{}{
		SourceKubelet: {}, SourceHostOS: {}, SourceKubeCluster: {}, SourcePrometheusApp: {},
		SourceMeshIstio: {}, SourceTracesServicegraph: {},
	}
	for _, metric := range DefaultCatalog() {
		for _, ref := range metric.Sources {
			_, ok := known[ref.ID]
			require.Truef(t, ok, "metric %s references unknown source %s", metric.Name, ref.ID)
		}
	}
}

func hasScope(metric CanonicalMetric, scope MetricScope) bool {
	for _, candidate := range metric.Scopes {
		if candidate == scope {
			return true
		}
	}
	return false
}

func catalogByName(catalog []CanonicalMetric) map[string]CanonicalMetric {
	out := make(map[string]CanonicalMetric, len(catalog))
	for _, metric := range catalog {
		out[metric.Name] = metric
	}
	return out
}
