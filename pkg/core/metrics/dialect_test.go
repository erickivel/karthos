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
	registry := NewDialectRegistry(PrometheusDialect, ServicegraphDialect, IstioDialect)

	require.Equal(t, "http.server.request.count", registry.Translate("http_requests_total"))
	require.Equal(t, "http.server.request.duration", registry.Translate("http_request_duration_seconds"))
	require.Equal(t, "servicegraph.request.count", registry.Translate("traces_service_graph_request_total"))
	require.Equal(t, "servicegraph.request.duration", registry.Translate("traces_service_graph_request_server"))
	require.Equal(t, "servicegraph.request.error.count", registry.Translate("traces_service_graph_request_failed_total"))
	require.Equal(t, "servicegraph.request.count", registry.Translate("istio_requests_total"))
	require.Equal(t, "servicegraph.request.duration", registry.Translate("istio_request_duration_milliseconds"))
}

func TestIstioDialectTranslatesProtocolCountersToQueryableCanonicals(t *testing.T) {
	for rawName, canonicalName := range map[string]string{
		"istio_request_messages_total":       "servicegraph.grpc.request.message.count",
		"istio_response_messages_total":      "servicegraph.grpc.response.message.count",
		"istio_tcp_connections_opened_total": "servicegraph.tcp.connection.open.count",
		"istio_tcp_connections_closed_total": "servicegraph.tcp.connection.close.count",
		"istio_tcp_sent_bytes_total":         "servicegraph.tcp.bytes.sent",
		"istio_tcp_received_bytes_total":     "servicegraph.tcp.bytes.received",
	} {
		translated, ok := IstioDialect.Translate(rawName)
		require.Truef(t, ok, "missing Istio translation for %s", rawName)
		require.Equal(t, canonicalName, translated)
	}
}

func TestSourceDialectRegistryTranslatesKubeletContainerMetrics(t *testing.T) {
	registry := NewSourceDialectRegistry(map[string]Dialect{SourceKubelet: KubeletDialect, SourceKubeCluster: KubeClusterDialect})

	require.Equal(t, "k8s.container.cpu.usage", registry.Translate("container.cpu.usage"))
	require.Equal(t, "k8s.container.memory.usage", registry.Translate("container.memory.usage"))
}

func TestSourceDialectRegistryTranslatesKubeClusterContainerSpecs(t *testing.T) {
	registry := NewSourceDialectRegistry(map[string]Dialect{SourceKubelet: KubeletDialect, SourceKubeCluster: KubeClusterDialect})

	require.Equal(t, "k8s.container.cpu.request", registry.Translate("k8s.container.cpu_request"))
	require.Equal(t, "k8s.container.cpu.limit", registry.Translate("k8s.container.cpu_limit"))
	require.Equal(t, "k8s.container.memory.request", registry.Translate("k8s.container.memory_request"))
	require.Equal(t, "k8s.container.memory.limit", registry.Translate("k8s.container.memory_limit"))
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
		"k8s.container.ready":                {},
		"k8s.container.restarts":             {},
		"k8s.pod.phase":                      {},
		"k8s.pod.status_reason":              {},
		"k8s.node.condition.ready":           {},
		"k8s.node.condition.memory_pressure": {},
		"k8s.node.condition.disk_pressure":   {},
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
