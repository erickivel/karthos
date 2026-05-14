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
		"k8s.pod.cpu.usage":                        {unit: UnitMillicores, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.pod.memory.working_set":               {unit: UnitBytes, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.node.cpu.usage":                       {unit: UnitMillicores, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.node.memory.working_set":              {unit: UnitBytes, category: CategoryResource, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"system.cpu.utilization":                   {unit: UnitPercent, category: CategoryResource, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
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
		"k8s.pod.phase":                            {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.pod.status_reason":                    {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"oksg.pod.health.status":                   {unit: UnitCount, category: CategoryHealth, sources: nil},
		"oksg.container.health.status":             {unit: UnitCount, category: CategoryHealth, sources: nil},
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
		"servicegraph.tcp.bytes.sent":              {unit: UnitBytesPerSecond, category: CategoryTraffic, sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}}},
		"servicegraph.tcp.bytes.received":          {unit: UnitBytesPerSecond, category: CategoryTraffic, sources: []SourceRef{{ID: SourceMeshIstio, Priority: 1}}},
		"k8s.pod.network.io":                       {unit: UnitBytesPerSecond, category: CategoryTraffic, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.pod.network.errors":                   {unit: UnitPerSecond, category: CategoryError, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.node.network.io":                      {unit: UnitBytesPerSecond, category: CategoryTraffic, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.node.network.errors":                  {unit: UnitPerSecond, category: CategoryError, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.pod.filesystem.usage":                 {unit: UnitBytes, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.pod.filesystem.capacity":              {unit: UnitBytes, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.pod.filesystem.available":             {unit: UnitBytes, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.node.filesystem.usage":                {unit: UnitBytes, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.node.filesystem.capacity":             {unit: UnitBytes, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.node.filesystem.available":            {unit: UnitBytes, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.container.filesystem.usage":           {unit: UnitBytes, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.container.filesystem.capacity":        {unit: UnitBytes, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.container.filesystem.available":       {unit: UnitBytes, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.volume.inodes":                        {unit: UnitCount, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
		"k8s.node.allocatable_ephemeral_storage":   {unit: UnitBytes, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.node.allocatable_pods":                {unit: UnitCount, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.container.storage.request":            {unit: UnitBytes, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.container.storage.limit":              {unit: UnitBytes, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.container.ephemeral_storage.request":  {unit: UnitBytes, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.container.ephemeral_storage.limit":    {unit: UnitBytes, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.cronjob.active_jobs":                  {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.daemonset.current_scheduled_nodes":    {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.daemonset.misscheduled_nodes":         {unit: UnitCount, category: CategoryError, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.hpa.max_replicas":                     {unit: UnitCount, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.hpa.min_replicas":                     {unit: UnitCount, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.hpa.current_replicas":                 {unit: UnitCount, category: CategoryResource, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.hpa.desired_replicas":                 {unit: UnitCount, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.job.active_pods":                      {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.job.desired_successful_pods":          {unit: UnitCount, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.job.failed_pods":                      {unit: UnitCount, category: CategoryError, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.job.max_parallel_pods":                {unit: UnitCount, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.job.successful_pods":                  {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.replicaset.available":                 {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.replication_controller.desired":       {unit: UnitCount, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.replication_controller.available":     {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.resource_quota.hard_limit":            {unit: UnitCount, category: CategoryCapacity, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.resource_quota.used":                  {unit: UnitCount, category: CategoryResource, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.statefulset.current_pods":             {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"k8s.statefulset.updated_pods":             {unit: UnitCount, category: CategoryHealth, sources: []SourceRef{{ID: SourceKubeCluster, Priority: 1}}},
		"system.cpu.time":                          {unit: UnitNanoseconds, category: CategoryResource, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
		"system.disk.io_time":                      {unit: UnitNanoseconds, category: CategoryResource, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
		"system.disk.weighted_io_time":             {unit: UnitNanoseconds, category: CategoryResource, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
		"system.disk.merged":                       {unit: UnitPerSecond, category: CategoryResource, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
		"system.disk.pending_operations":           {unit: UnitCount, category: CategoryResource, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
		"system.filesystem.inodes.usage":           {unit: UnitCount, category: CategoryCapacity, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
		"system.paging.faults":                     {unit: UnitPerSecond, category: CategoryResource, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
		"system.network.connections":               {unit: UnitCount, category: CategoryTraffic, sources: []SourceRef{{ID: SourceHostOS, Priority: 1}}},
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
	require.Equal(t, []entities.EntityKind{entities.KindContainer}, catalog["oksg.container.health.status"].AppliesTo)
	require.Equal(t, []entities.EntityKind{entities.KindPod}, catalog["k8s.pod.cpu.usage"].AppliesTo)
	require.Equal(t, []entities.EntityKind{entities.KindNode}, catalog["k8s.node.cpu.usage"].AppliesTo)
	require.Equal(t, []entities.EntityKind{entities.KindPVC}, catalog["k8s.volume.inodes.used"].AppliesTo)
	require.Equal(t, []entities.EntityKind{entities.KindService}, catalog["servicegraph.request.count"].AppliesTo)
	require.Equal(t, []entities.EntityKind{entities.KindHPA}, catalog["k8s.hpa.current_replicas"].AppliesTo)
	require.Equal(t, []entities.EntityKind{entities.KindResourceQuota}, catalog["k8s.resource_quota.used"].AppliesTo)
	require.Empty(t, catalog["servicegraph.request.duration"].AppliesTo)
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
	"servicegraph.request.error.ratio": {},
	"oksg.pod.health.status":           {},
	"oksg.container.health.status":     {},
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
