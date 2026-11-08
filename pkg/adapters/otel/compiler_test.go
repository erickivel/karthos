package otel

import (
	"regexp"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	collectorspec "github.com/erickivel/karthos/pkg/collector/spec"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/discovery/profile"
	"github.com/erickivel/karthos/pkg/sources/kubecluster"
)

func TestCompileGatewayStampsRawKubeClusterNodeConditions(t *testing.T) {
	plan := collectorspec.CollectionPlan{ClusterName: "kind-test", Namespace: "karthos-system"}
	kubecluster.Contribute(&plan, profile.SourceCapability{}, true)

	deployment, err := NewCompiler().Compile(plan)
	require.NoError(t, err)
	processor := deployment.GatewayConfig.Processors[karthosSourceTaggerProcessor].(map[string]any)
	statements := processor["metric_statements"].([]map[string]any)[0]["statements"].([]string)
	matched := false
	for _, statement := range statements {
		if strings.Contains(statement, `metric.name == "k8s.node.condition"`) {
			matched = true
			require.Contains(t, statement, `set(datapoint.attributes["karthos_source"], "kube-cluster")`)
			require.Contains(t, statement, `datapoint.attributes["karthos_source"] == nil`)
		}
	}
	require.True(t, matched, "expected the raw node-condition metric to receive a kube-cluster source stamp")
}

func TestCompileAddsFixedJSONStdoutServiceTelemetry(t *testing.T) {
	deployment, err := NewCompiler().Compile(collectorspec.CollectionPlan{
		ClusterName: "kind-test",
		Namespace:   "karthos-system",
	})
	require.NoError(t, err)

	for _, config := range []CollectorConfig{deployment.AgentConfig, deployment.GatewayConfig} {
		encoded, err := yaml.Marshal(config)
		require.NoError(t, err)
		require.Contains(t, string(encoded), "telemetry:\n    logs:\n      encoding: json\n      level: info\n      output_paths:\n      - stdout\n")
	}
}

func TestCompileUsesDerivedServiceGraphOnlyWhenRequested(t *testing.T) {
	deployment, err := NewCompiler().Compile(collectorspec.CollectionPlan{
		ClusterName: "kind-test",
		Namespace:   "karthos-system",
		Agent:       collectorspec.AgentRequirements{OTLP: collectorspec.OTLPReceiver{Traces: true}},
		Gateway: collectorspec.GatewayRequirements{
			OTLP:         collectorspec.OTLPReceiver{Traces: true},
			ServiceGraph: &collectorspec.ServiceGraph{SourceID: metrics.SourceTracesServicegraph},
		},
	})
	require.NoError(t, err)
	require.Contains(t, deployment.GatewayConfig.Connectors, "service_graph")
	require.Equal(t, []string{otlpGRPCExporter, "service_graph"}, deployment.GatewayConfig.Service.Pipelines["traces"].Exporters)
	require.Equal(t, []string{"memory_limiter", serviceGraphNamespaceProcessor, "batch"}, deployment.GatewayConfig.Service.Pipelines["traces"].Processors)
	require.Contains(t, deployment.GatewayConfig.Processors, serviceGraphNamespaceProcessor)
}

func TestCompileOmitsCollectorDebugByDefault(t *testing.T) {
	deployment, err := NewCompiler().Compile(collectorspec.CollectionPlan{
		ClusterName: "kind-test",
		Namespace:   "karthos-system",
		Agent:       collectorspec.AgentRequirements{OTLP: collectorspec.OTLPReceiver{Metrics: true, Traces: true}},
		Gateway:     collectorspec.GatewayRequirements{OTLP: collectorspec.OTLPReceiver{Metrics: true, Traces: true}},
	})
	require.NoError(t, err)
	require.NotContains(t, deployment.AgentConfig.Exporters, "debug/karthos")
	require.NotContains(t, deployment.GatewayConfig.Exporters, "debug/karthos")
}

func TestCompileAddsFixedCollectorDebugToEveryEnabledPipeline(t *testing.T) {
	deployment, err := NewCompilerWithOptions(CompilerOptions{DebugEnabled: true}).Compile(collectorspec.CollectionPlan{
		ClusterName: "kind-test",
		Namespace:   "karthos-system",
		Agent:       collectorspec.AgentRequirements{OTLP: collectorspec.OTLPReceiver{Metrics: true, Traces: true}},
		Gateway:     collectorspec.GatewayRequirements{OTLP: collectorspec.OTLPReceiver{Metrics: true, Traces: true}},
	})
	require.NoError(t, err)

	wantExporter := map[string]any{
		"verbosity":           "detailed",
		"use_internal_logger": false,
		"output_paths":        []string{"stdout"},
	}
	for _, config := range []CollectorConfig{deployment.AgentConfig, deployment.GatewayConfig} {
		require.Equal(t, wantExporter, config.Exporters["debug/karthos"])
		for pipelineName, pipeline := range config.Service.Pipelines {
			require.Containsf(t, pipeline.Exporters, "debug/karthos", "expected %s pipeline to emit debug batches", pipelineName)
		}
	}
}

func TestCompileGatewayUsesReservedBackendEndpointEnvironment(t *testing.T) {
	deployment, err := NewCompiler().Compile(collectorspec.CollectionPlan{
		ClusterName: "kind-test",
		Namespace:   "karthos-system",
	})
	require.NoError(t, err)

	exporter := deployment.GatewayConfig.Exporters[otlpGRPCExporter].(map[string]any)
	require.Equal(t, "${env:KARTHOS_BACKEND_GRPC_ENDPOINT}", exporter["endpoint"])
}

func TestCompileAgentUsesReservedGatewayEndpointEnvironment(t *testing.T) {
	deployment, err := NewCompiler().Compile(collectorspec.CollectionPlan{
		ClusterName: "kind-test",
		Namespace:   "karthos-system",
	})
	require.NoError(t, err)

	exporter := deployment.AgentConfig.Exporters[otlpGRPCExporter].(map[string]any)
	require.Equal(t, "${env:KARTHOS_GATEWAY_GRPC_ENDPOINT}", exporter["endpoint"])
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

func TestPrometheusAppScrapesAtFifteenSeconds(t *testing.T) {
	config := prometheusReceiver(&collectorspec.Prometheus{SourceID: metrics.SourcePrometheusApp, Targets: []profile.PrometheusTarget{{Namespace: "demo", ServiceName: "api", Port: 8080, Path: "/metrics"}}})
	scrapes := config["config"].(map[string]any)["scrape_configs"].([]map[string]any)
	require.Len(t, scrapes, 1)
	require.Equal(t, "15s", scrapes[0]["scrape_interval"])
}

func TestCompileGatewayDisablesKubeClusterDefaultsOutsideThePoCAllowlist(t *testing.T) {
	deployment, err := NewCompiler().Compile(collectorspec.CollectionPlan{
		ClusterName: "kind-test",
		Namespace:   "karthos-system",
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
