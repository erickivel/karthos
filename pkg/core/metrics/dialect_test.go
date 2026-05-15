package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDialectRegistryFirstMatchWinsAndFallsBackToIdentity(t *testing.T) {
	registry := NewDialectRegistry(
		DialectFromMap(map[string]string{
			"http_requests_total": "custom.http.count",
		}),
		DialectFromMap(map[string]string{
			"http_requests_total": "http.server.request.count",
		}),
	)

	require.Equal(t, "custom.http.count", registry.Translate("http_requests_total"))
	require.Equal(t, "k8s.pod.cpu.usage", registry.Translate("k8s.pod.cpu.usage"))
}

func TestBuiltInDialectsTranslateKnownMetrics(t *testing.T) {
	registry := NewDialectRegistry(PrometheusDialect, SpanmetricsDialect, ServicegraphDialect, IstioDialect)

	require.Equal(t, "http.server.request.count", registry.Translate("http_requests_total"))
	require.Equal(t, "http.server.request.duration", registry.Translate("http_request_duration_seconds"))
	require.Equal(t, "rpc.server.request.count", registry.Translate("traces.span.metrics.calls"))
	require.Equal(t, "rpc.server.duration", registry.Translate("traces.span.metrics.duration"))
	require.Equal(t, "servicegraph.request.count", registry.Translate("traces.service.graph.request.total"))
	require.Equal(t, "servicegraph.request.duration", registry.Translate("traces.service.graph.request.server"))
	require.Equal(t, "servicegraph.request.error.count", registry.Translate("traces.service.graph.request_failed_total"))
	require.Equal(t, "servicegraph.request.count", registry.Translate("istio_requests_total"))
	require.Equal(t, "servicegraph.request.duration", registry.Translate("istio_request_duration_milliseconds"))
	require.Equal(t, "servicegraph.request.body.size", registry.Translate("istio_request_bytes"))
	require.Equal(t, "servicegraph.response.body.size", registry.Translate("istio_response_bytes"))
	require.Equal(t, "servicegraph.grpc.request.message.count", registry.Translate("istio_request_messages_total"))
	require.Equal(t, "servicegraph.grpc.response.message.count", registry.Translate("istio_response_messages_total"))
	require.Equal(t, "servicegraph.tcp.connection.open.count", registry.Translate("istio_tcp_connections_opened_total"))
	require.Equal(t, "servicegraph.tcp.connection.close.count", registry.Translate("istio_tcp_connections_closed_total"))
	require.Equal(t, "servicegraph.tcp.bytes.sent", registry.Translate("istio_tcp_sent_bytes_total"))
	require.Equal(t, "servicegraph.tcp.bytes.received", registry.Translate("istio_tcp_received_bytes_total"))
}

func TestDefaultSourceDialectsTranslateKubeletContainerMetrics(t *testing.T) {
	registry := NewDialectRegistryFromSources(DefaultLogicalSources())

	require.Equal(t, "k8s.container.cpu.usage", registry.Translate("container.cpu.usage"))
	require.Equal(t, "k8s.container.memory.usage", registry.Translate("container.memory.usage"))
	require.Equal(t, "k8s.container.filesystem.usage", registry.Translate("container.filesystem.usage"))
	require.Equal(t, "k8s.container.filesystem.capacity", registry.Translate("container.filesystem.capacity"))
	require.Equal(t, "k8s.container.filesystem.available", registry.Translate("container.filesystem.available"))
}

func TestDefaultSourceDialectsTranslateKubeClusterContainerSpecs(t *testing.T) {
	registry := NewDialectRegistryFromSources(DefaultLogicalSources())

	require.Equal(t, "k8s.container.cpu.request", registry.Translate("k8s.container.cpu_request"))
	require.Equal(t, "k8s.container.cpu.limit", registry.Translate("k8s.container.cpu_limit"))
	require.Equal(t, "k8s.container.memory.request", registry.Translate("k8s.container.memory_request"))
	require.Equal(t, "k8s.container.memory.limit", registry.Translate("k8s.container.memory_limit"))
	require.Equal(t, "k8s.container.storage.request", registry.Translate("k8s.container.storage_request"))
	require.Equal(t, "k8s.container.storage.limit", registry.Translate("k8s.container.storage_limit"))
	require.Equal(t, "k8s.container.ephemeral_storage.request", registry.Translate("k8s.container.ephemeralstorage_request"))
	require.Equal(t, "k8s.container.ephemeral_storage.limit", registry.Translate("k8s.container.ephemeralstorage_limit"))
}

// TestKubeClusterDialect_CoverageForCatalogSources walks every catalog entry
// whose Sources include SourceKubeCluster and asserts each canonical name is
// reachable from the receiver — either via a KubeClusterDialect translation
// (wire-side underscore name → dotted canonical) or as a documented passthrough
// where the receiver emits the catalog name directly.
//
// Adding a new SourceKubeCluster catalog metric without either a dialect entry
// or a passthrough acknowledgement here forces an explicit decision rather than
// silently producing zero datapoints in production.
func TestKubeClusterDialect_CoverageForCatalogSources(t *testing.T) {
	// kubeClusterPassthroughs are canonical names the k8s_cluster receiver
	// emits with the exact catalog spelling — no dialect translation needed.
	// Keep this list aligned with the comment block on KubeClusterDialect.
	kubeClusterPassthroughs := map[string]struct{}{
		"k8s.container.ready":                    {},
		"k8s.container.restarts":                 {},
		"k8s.namespace.phase":                    {},
		"k8s.pod.phase":                          {},
		"k8s.pod.status_reason":                  {},
		"k8s.replicaset.desired":                 {},
		"k8s.replicaset.available":               {},
		"k8s.replication_controller.desired":     {},
		"k8s.replication_controller.available":   {},
		"k8s.deployment.desired":                 {},
		"k8s.deployment.available":               {},
		"k8s.cronjob.active_jobs":                {},
		"k8s.daemonset.desired_scheduled_nodes":  {},
		"k8s.daemonset.current_scheduled_nodes":  {},
		"k8s.daemonset.misscheduled_nodes":       {},
		"k8s.daemonset.ready_nodes":              {},
		"k8s.statefulset.desired_pods":           {},
		"k8s.statefulset.ready_pods":             {},
		"k8s.statefulset.current_pods":           {},
		"k8s.statefulset.updated_pods":           {},
		"k8s.hpa.max_replicas":                   {},
		"k8s.hpa.min_replicas":                   {},
		"k8s.hpa.current_replicas":               {},
		"k8s.hpa.desired_replicas":               {},
		"k8s.job.active_pods":                    {},
		"k8s.job.desired_successful_pods":        {},
		"k8s.job.failed_pods":                    {},
		"k8s.job.max_parallel_pods":              {},
		"k8s.job.successful_pods":                {},
		"k8s.resource_quota.hard_limit":          {},
		"k8s.resource_quota.used":                {},
		"k8s.node.condition.ready":               {},
		"k8s.node.condition.memory_pressure":     {},
		"k8s.node.condition.disk_pressure":       {},
		"k8s.node.allocatable_cpu":               {},
		"k8s.node.allocatable_memory":            {},
		"k8s.node.allocatable_ephemeral_storage": {},
		"k8s.node.allocatable_pods":              {},
	}

	dialectCanonicals := make(map[string]struct{}, len(KubeClusterDialect))
	for _, canonical := range KubeClusterDialect {
		dialectCanonicals[canonical] = struct{}{}
	}

	// Derived/composite metrics have no Sources entry — they are computed at
	// query time from other metrics — so they are out of scope for this test.
	for _, m := range DefaultCatalog() {
		hasKubeCluster := false
		for _, s := range m.Sources {
			if s.ID == SourceKubeCluster {
				hasKubeCluster = true
				break
			}
		}
		if !hasKubeCluster {
			continue
		}
		_, viaDialect := dialectCanonicals[m.Name]
		_, viaPassthrough := kubeClusterPassthroughs[m.Name]
		require.Truef(t, viaDialect || viaPassthrough,
			"catalog metric %q lists SourceKubeCluster but is neither in KubeClusterDialect nor in the passthrough allow-list — add a dialect entry or extend kubeClusterPassthroughs in this test",
			m.Name)
	}
}
