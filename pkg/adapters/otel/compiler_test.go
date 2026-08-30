package otel

import (
	"regexp"
	"testing"

	"github.com/stretchr/testify/require"

	collectorspec "github.com/erickivel/oksg/pkg/collector/spec"
	"github.com/erickivel/oksg/pkg/core/metrics"
)

func TestCompileUsesDerivedServiceGraphOnlyWhenRequested(t *testing.T) {
	deployment, err := NewCompiler().Compile(collectorspec.CollectionPlan{
		ClusterName: "kind-test",
		Namespace:   "oksg-system",
		Agent:       collectorspec.AgentRequirements{OTLP: collectorspec.OTLPReceiver{Traces: true}},
		Gateway: collectorspec.GatewayRequirements{
			OTLP:         collectorspec.OTLPReceiver{Traces: true},
			ServiceGraph: &collectorspec.ServiceGraph{SourceID: metrics.SourceTracesServicegraph},
		},
	})
	require.NoError(t, err)
	require.Contains(t, deployment.GatewayConfig.Connectors, "service_graph")
	require.Equal(t, []string{otlpGRPCExporter, "service_graph"}, deployment.GatewayConfig.Service.Pipelines["traces"].Exporters)
}

func TestCompileProvidesDevelopmentDebugExporterSpecification(t *testing.T) {
	deployment, err := NewCompiler().Compile(collectorspec.CollectionPlan{
		ClusterName: "kind-test",
		Namespace:   "oksg-system",
	})
	require.NoError(t, err)
	require.Equal(t, "debug/oksg", deployment.DebugExporter.ID)
	require.Equal(t, "detailed", deployment.DebugExporter.Config["verbosity"])
	require.Equal(t, false, deployment.DebugExporter.Config["use_internal_logger"])
	require.Equal(t, []string{"stdout"}, deployment.DebugExporter.Config["output_paths"])
}

func TestCompileGatewayUsesReservedBackendEndpointEnvironment(t *testing.T) {
	deployment, err := NewCompiler().Compile(collectorspec.CollectionPlan{
		ClusterName: "kind-test",
		Namespace:   "oksg-system",
	})
	require.NoError(t, err)

	exporter := deployment.GatewayConfig.Exporters[otlpGRPCExporter].(map[string]any)
	require.Equal(t, "${env:OKSG_BACKEND_GRPC_ENDPOINT}", exporter["endpoint"])
}

func TestCompileRetainsIstioProtocolCounters(t *testing.T) {
	config := istioReceiver(&collectorspec.Istio{SourceID: metrics.SourceMeshIstio, Flavor: "istio", MetricsPort: 15020, MetricsPath: "/stats/prometheus"})
	scrapes := config["config"].(map[string]any)["scrape_configs"].([]map[string]any)
	metricRelabels := scrapes[0]["metric_relabel_configs"].([]map[string]any)
	matcher := regexp.MustCompile("^" + metricRelabels[0]["regex"].(string) + "$")
	for _, metricName := range []string{"istio_request_messages_total", "istio_response_messages_total", "istio_tcp_connections_opened_total", "istio_tcp_connections_closed_total", "istio_tcp_sent_bytes_total", "istio_tcp_received_bytes_total"} {
		require.Truef(t, matcher.MatchString(metricName), "mesh scrape must retain %s", metricName)
	}
}

func TestCompileGatewayDisablesKubeClusterDefaultsOutsideThePoCAllowlist(t *testing.T) {
	deployment, err := NewCompiler().Compile(collectorspec.CollectionPlan{
		ClusterName: "kind-test",
		Namespace:   "oksg-system",
		Gateway: collectorspec.GatewayRequirements{KubeCluster: &collectorspec.KubeCluster{
			SourceID: metrics.SourceKubeCluster,
			EnabledMetricNames: []string{
				"k8s.container.cpu_limit", "k8s.container.cpu_request", "k8s.container.memory_limit", "k8s.container.memory_request",
				"k8s.container.ready", "k8s.container.restarts", "k8s.node.condition", "k8s.pod.phase", "k8s.pod.status_reason",
			},
		}},
	})
	require.NoError(t, err)

	receiver := deployment.GatewayConfig.Receivers["k8s_cluster"].(map[string]any)
	configured := receiver["metrics"].(map[string]any)
	for _, metricName := range []string{"k8s.container.cpu_limit", "k8s.container.cpu_request", "k8s.container.memory_limit", "k8s.container.memory_request", "k8s.container.ready", "k8s.container.restarts", "k8s.node.condition", "k8s.pod.phase", "k8s.pod.status_reason"} {
		setting, ok := configured[metricName].(map[string]any)
		require.Truef(t, ok, "expected %s to have an explicit receiver setting", metricName)
		require.Equalf(t, true, setting["enabled"], "expected %s to be enabled", metricName)
	}
	for _, metricName := range []string{"k8s.container.ephemeralstorage_limit", "k8s.deployment.available", "k8s.resource_quota.used", "k8s.statefulset.ready_pods"} {
		setting, ok := configured[metricName].(map[string]any)
		require.Truef(t, ok, "expected %s to have an explicit receiver setting", metricName)
		require.Equalf(t, false, setting["enabled"], "expected %s to be disabled", metricName)
	}
}
