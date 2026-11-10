package metrics

import (
	"testing"

	"github.com/erickivel/karthos/pkg/core/entities"
	"github.com/stretchr/testify/require"
)

func TestDefaultCatalogIncludesRequiredMetrics(t *testing.T) {
	type metricExpectation struct {
		unit     Unit
		category MetricCategory
		sources  []SourceRef
	}

	required := map[string]metricExpectation{
		"k8s.pod.cpu.usage":                  {unit: UnitCores, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.pod.memory.working_set":         {unit: UnitBytes, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.node.cpu.usage":                 {unit: UnitCores, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.node.memory.working_set":        {unit: UnitBytes, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"system.cpu.utilization":             {unit: UnitRatio, category: CategoryResource, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
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
		"container.cpu.usage":                {unit: UnitCores, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"container.memory.usage":             {unit: UnitBytes, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.container.ready":                {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.container.restart.count":        {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.pod.volume.available":           {},
		"k8s.pod.volume.capacity":            {},
		"k8s.pod.volume.inode.free":          {},
		"k8s.pod.volume.inode.used":          {},
		"k8s.pod.status.phase":               {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.pod.status.reason":              {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.node.condition.ready":           {},
		"k8s.node.condition.memory_pressure": {},
		"k8s.node.condition.disk_pressure":   {},
		"k8s.pod.volume.inode.count":         {unit: UnitCount, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
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

func TestDefaultCatalogMetricsDeclareStorageSemantics(t *testing.T) {
	for _, metric := range DefaultCatalog() {
		require.NotEmptyf(t, metric.Instrument, "catalog metric %s has no storage instrument", metric.Name)
		require.NotEmptyf(t, metric.StoredUnit, "catalog metric %s has no stored base unit", metric.Name)
	}
}

func TestDefaultCatalogDeclaresProviderNeutralStorageSemantics(t *testing.T) {
	catalog := catalogByName(DefaultCatalog())

	require.Equal(t, InstrumentGauge, catalog["system.memory.usage"].Instrument)
	require.Equal(t, UnitBytes, catalog["system.memory.usage"].StoredUnit)
	require.Equal(t, InstrumentCounter, catalog["servicegraph.request.count"].Instrument)
	require.Equal(t, UnitCount, catalog["servicegraph.request.count"].StoredUnit)
	require.Equal(t, InstrumentHistogram, catalog["servicegraph.request.duration"].Instrument)
	require.Equal(t, UnitSeconds, catalog["servicegraph.request.duration"].StoredUnit)
}

func TestDefaultCatalogDeepCopiesEveryMutableField(t *testing.T) {
	first := CatalogByName()["servicegraph.request.count"]
	first.Scopes[0] = ScopeEdge
	first.Sources[0].Priority = 99

	second := CatalogByName()["servicegraph.request.count"]
	require.Equal(t, ScopeEntity, second.Scopes[0])
	require.Equal(t, 1, second.Sources[0].Priority)
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

	require.Equal(t, []entities.EntityKind{entities.KindContainer}, catalog["container.cpu.usage"].AppliesTo)
	require.Equal(t, []entities.EntityKind{entities.KindContainer}, catalog["k8s.container.ready"].AppliesTo)
	require.Equal(t, []entities.EntityKind{entities.KindPod}, catalog["k8s.pod.cpu.usage"].AppliesTo)
	require.Equal(t, []entities.EntityKind{entities.KindNode}, catalog["k8s.node.cpu.usage"].AppliesTo)
	require.Equal(t, []entities.EntityKind{entities.KindPVC, entities.KindPod}, catalog["k8s.pod.volume.inode.used"].AppliesTo)
	require.Equal(t, []entities.EntityKind{entities.KindService}, catalog["servicegraph.request.count"].AppliesTo)
	require.Empty(t, catalog["servicegraph.request.duration"].AppliesTo)
}

func TestDefaultCatalogEntriesDeclareSources(t *testing.T) {
	for _, metric := range DefaultCatalog() {
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
