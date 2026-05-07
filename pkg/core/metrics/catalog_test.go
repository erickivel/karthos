package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultCatalogIncludesRequiredMetrics(t *testing.T) {
	type metricExpectation struct {
		unit     Unit
		category MetricCategory
		sources  []SourceRef
	}

	required := map[string]metricExpectation{
		"k8s.pod.cpu.usage":                        {unit: UnitMillicores, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.pod.memory.working_set":               {unit: UnitBytes, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.node.cpu.usage":                       {unit: UnitMillicores, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.node.memory.working_set":              {unit: UnitBytes, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"system.cpu.utilization":                   {unit: UnitMillicores, category: CategoryResource, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
		"system.memory.usage":                      {unit: UnitBytes, category: CategoryResource, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
		"k8s.deployment.desired":                   {unit: UnitCount, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"system.disk.io.read":                      {},
		"system.disk.io.write":                     {},
		"servicegraph.request.count":               {},
		"http.server.request.duration":             {},
		"servicegraph.request.duration":            {},
		"rpc.server.duration":                      {},
		"rpc.server.request.count":                 {},
		"servicegraph.request.error.count":         {},
		"servicegraph.request.error.ratio":         {},
		"servicegraph.request.body.size":           {},
		"servicegraph.response.body.size":          {},
		"servicegraph.grpc.request.message.count":  {},
		"servicegraph.grpc.response.message.count": {},
		"servicegraph.tcp.connection.open.count":   {},
		"servicegraph.tcp.connection.close.count":  {},
		"k8s.container.cpu.request":                {},
		"k8s.container.cpu.limit":                  {},
		"k8s.container.memory.request":             {},
		"k8s.container.memory.limit":               {},
		"k8s.container.cpu.usage":                  {unit: UnitMillicores, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.container.memory.usage":               {unit: UnitBytes, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.container.ready":                      {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.container.restarts":                   {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.replicaset.desired":                   {},
		"system.network.io.receive":                {},
		"system.network.io.transmit":               {},
		"system.network.errors":                    {unit: UnitPerSecond, category: CategoryError, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
		"system.network.dropped":                   {unit: UnitPerSecond, category: CategoryError, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
		"system.network.packets":                   {unit: UnitPerSecond, category: CategoryTraffic, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
		"system.disk.operations":                   {unit: UnitPerSecond, category: CategoryResource, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
		"system.disk.operation_time":               {unit: UnitNanoseconds, category: CategoryResource, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
		"k8s.volume.available":                     {},
		"k8s.volume.capacity":                      {},
		"k8s.volume.inodes.free":                   {},
		"k8s.volume.inodes.used":                   {},
		"system.filesystem.usage":                  {},
		"system.filesystem.utilization":            {},
		"system.cpu.load_average.1m":               {},
		"system.cpu.load_average.5m":               {},
		"system.cpu.load_average.15m":              {},
		"system.paging.usage":                      {},
		"system.paging.operations":                 {},
		"k8s.namespace.phase":                      {},
		"k8s.node.condition.ready":                 {},
		"k8s.node.condition.memory_pressure":       {},
		"k8s.node.condition.disk_pressure":         {},
		"k8s.node.allocatable_cpu":                 {},
		"k8s.node.allocatable_memory":              {},
		"k8s.deployment.available":                 {},
		"k8s.daemonset.desired_scheduled_nodes":    {},
		"k8s.daemonset.ready_nodes":                {},
		"k8s.statefulset.desired_pods":             {},
		"k8s.statefulset.ready_pods":               {},
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
		"k8s.node.cpu.capacity":               {},
		"k8s.node.cpu.cores":                  {},
		"k8s.node.cpu.frequency":              {},
		"k8s.node.memory.capacity":            {},
		"k8s.node.disk.capacity":              {},
		"k8s.node.network.bandwidth.receive":  {},
		"k8s.node.network.bandwidth.transmit": {},
		"k8s.pod.network.io.receive":          {},
		"k8s.pod.network.io.transmit":         {},
		"k8s.pod.phase":                       {},
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

// servicegraph.request.error.ratio is query-derived (see metric_store.buildPromQL)
// and never arrives as an OTLP sample, so it intentionally has no source.
var catalogSourcelessExceptions = map[string]struct{}{
	"servicegraph.request.error.ratio": {},
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
	known := make(map[string]struct{}, len(DefaultLogicalSources()))
	for _, source := range DefaultLogicalSources() {
		known[source.ID] = struct{}{}
	}
	for _, metric := range DefaultCatalog() {
		for _, ref := range metric.Sources {
			_, ok := known[ref.ID]
			require.Truef(t, ok, "metric %s references unknown source %s", metric.Name, ref.ID)
		}
	}
}
