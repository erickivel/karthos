package otel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

func TestPlanIncludesPrometheusReceiverWhenTargetsExist(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources: []profile.ObservabilitySource{
			{
				Type:    "prometheus",
				Present: true,
				PrometheusTargets: []profile.PrometheusTarget{
					{
						ServiceName: "gateway-go",
						Namespace:   "oksg-demo",
						Port:        8080,
						Path:        "/metrics",
						Reachable:   true,
					},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "kind-test", plan.ClusterName)
	require.Equal(t, "oksg-system", plan.Namespace)
	require.Contains(t, plan.AgentConfig.Extensions, "health_check")
	require.Contains(t, plan.GatewayConfig.Extensions, "health_check")
	require.Contains(t, plan.AgentConfig.Service.Extensions, "health_check")
	require.Contains(t, plan.GatewayConfig.Service.Extensions, "health_check")
	require.Contains(t, plan.AgentConfig.Receivers, "kubeletstats")
	require.Contains(t, plan.AgentConfig.Receivers, "prometheus")
	require.Contains(t, plan.GatewayConfig.Receivers, "otlp")
}

func TestPlanPrometheusScrapeStampsAppSource(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources: []profile.ObservabilitySource{
			{
				Type:    "prometheus",
				Present: true,
				PrometheusTargets: []profile.PrometheusTarget{
					{
						ServiceName: "gateway-go",
						Namespace:   "oksg-demo",
						Port:        8080,
						Path:        "/metrics",
						Reachable:   true,
					},
				},
			},
		},
	})
	require.NoError(t, err)

	prometheus := plan.AgentConfig.Receivers["prometheus"].(map[string]any)
	config := prometheus["config"].(map[string]any)
	scrapeConfigs := config["scrape_configs"].([]map[string]any)
	require.Len(t, scrapeConfigs, 1)

	metricRelabels := scrapeConfigs[0]["metric_relabel_configs"].([]map[string]any)
	require.Contains(t, metricRelabels, map[string]any{
		"target_label": "oksg_source",
		"replacement":  metrics.SourcePrometheusApp,
	})
}

func TestPlanIncludesIstioMeshScrapeJobWhenMeshSourceExists(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources: []profile.ObservabilitySource{
			{
				Name:    "istio",
				Type:    "istio",
				Present: true,
				Mesh: &profile.MeshSource{
					Flavor:             "istio",
					Mode:               "sidecar",
					InjectedNamespaces: []string{"oksg-demo"},
					MetricsPort:        15020,
					MetricsPath:        "/stats/prometheus",
				},
			},
		},
	})
	require.NoError(t, err)
	require.Contains(t, plan.AgentConfig.Receivers, "prometheus")
	require.Contains(t, plan.AgentConfig.Service.Pipelines["metrics"].Receivers, "prometheus")

	prometheus := plan.AgentConfig.Receivers["prometheus"].(map[string]any)
	config := prometheus["config"].(map[string]any)
	scrapeConfigs := config["scrape_configs"].([]map[string]any)
	require.Len(t, scrapeConfigs, 1)
	meshJob := scrapeConfigs[0]
	require.Equal(t, "mesh-istio", meshJob["job_name"])
	require.Equal(t, "/stats/prometheus", meshJob["metrics_path"])

	relabelConfigs := meshJob["relabel_configs"].([]map[string]any)
	require.Contains(t, relabelConfigs, map[string]any{
		"source_labels": []string{"__meta_kubernetes_pod_container_name"},
		"action":        "keep",
		"regex":         "istio-proxy",
	})
	require.Contains(t, relabelConfigs, map[string]any{
		"source_labels": []string{"__meta_kubernetes_namespace"},
		"action":        "keep",
		"regex":         "oksg-demo",
	})

	metricRelabels := meshJob["metric_relabel_configs"].([]map[string]any)
	require.Contains(t, metricRelabels, map[string]any{
		"source_labels": []string{"reporter"},
		"action":        "keep",
		"regex":         "destination",
	})
	require.Contains(t, metricRelabels, map[string]any{
		"target_label": "oksg_source",
		"replacement":  "mesh.istio",
	})
	require.Contains(t, metricRelabels, map[string]any{
		"source_labels": []string{"source_canonical_service"},
		"target_label":  "client",
	})
	require.Contains(t, metricRelabels, map[string]any{
		"source_labels": []string{"destination_canonical_service"},
		"target_label":  "server",
	})
}

func TestPlanFallsBackToClusterIDWhenClusterNameEmpty(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID: "abc123hash",
		Sources: []profile.ObservabilitySource{
			{
				Type:    "prometheus",
				Present: false,
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, "abc123hash", plan.ClusterName)
	require.Contains(t, plan.AgentConfig.Extensions, "health_check")
}

func TestPlanFallsBackToDefaultClusterNameWhenBothEmpty(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		Sources: []profile.ObservabilitySource{
			{
				Type:    "prometheus",
				Present: false,
			},
		},
	})
	require.NoError(t, err)
	require.Equal(t, DefaultClusterName, plan.ClusterName)
}

func TestPlanFallsBackToKubeletstatsOnlyWhenNoPrometheusSourcesExist(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources: []profile.ObservabilitySource{
			{
				Type:    "prometheus",
				Present: false,
			},
		},
	})
	require.NoError(t, err)
	require.Contains(t, plan.AgentConfig.Receivers, "kubeletstats")
	require.NotContains(t, plan.AgentConfig.Receivers, "prometheus")
}

func TestPlanEnablesOTLPMetricsAndTracePipelinesWhenOTLPSourcesExist(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources: []profile.ObservabilitySource{
			{
				Name:    "application-otlp",
				Type:    "otlp",
				Present: true,
				OTLP: &profile.OTLPSource{
					MetricsEnabled: true,
					TracesEnabled:  true,
					GRPCEndpoint:   "0.0.0.0:4317",
					HTTPEndpoint:   "0.0.0.0:4318",
				},
			},
		},
	})
	require.NoError(t, err)
	require.Contains(t, plan.AgentConfig.Receivers, "otlp")
	require.Contains(t, plan.AgentConfig.Service.Pipelines, "traces")
	require.Contains(t, plan.GatewayConfig.Service.Pipelines, "traces")
	require.Contains(t, plan.GatewayConfig.Exporters, "otlp")
	require.NotContains(t, plan.GatewayConfig.Exporters, "debug")
	require.Equal(t, []string{"metric", "trace"}, plan.EnabledSignals)
	require.Equal(t, DefaultBackendGRPCEndpoint, plan.Backend.GRPCEndpoint)
}

func TestPlanRoutesGatewayToBackendInFallbackMode(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources: []profile.ObservabilitySource{
			{
				Type:    "prometheus",
				Present: false,
			},
		},
	})
	require.NoError(t, err)
	require.Contains(t, plan.GatewayConfig.Exporters, "otlp")
	require.Equal(t, DefaultBackendGRPCEndpoint, plan.Backend.GRPCEndpoint)
	require.Equal(t, []string{"metric"}, plan.EnabledSignals)
}

// TestPlanBaselineAlwaysIncludesHostmetricsAndKubeletstatsVolume verifies that
// hostmetrics is always present in the agent receivers, kubeletstats always
// includes "volume" in metric_groups, and k8s_cluster is always in the gateway.
func TestPlanBaselineAlwaysIncludesHostmetricsAndKubeletstatsVolume(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources:     []profile.ObservabilitySource{},
	})
	require.NoError(t, err)

	// hostmetrics always in agent receivers.
	require.Contains(t, plan.AgentConfig.Receivers, "hostmetrics")

	// kubeletstats metric_groups must include "volume".
	kubeletstats, ok := plan.AgentConfig.Receivers["kubeletstats"].(map[string]any)
	require.True(t, ok, "kubeletstats receiver must be map[string]any")
	groups, ok := kubeletstats["metric_groups"].([]string)
	require.True(t, ok, "metric_groups must be []string")
	require.Contains(t, groups, "volume")

	// hostmetrics must be in the agent metrics pipeline receivers.
	agentMetricsPipeline := plan.AgentConfig.Service.Pipelines["metrics"]
	require.Contains(t, agentMetricsPipeline.Receivers, "hostmetrics")

	// hostmetrics cpu scraper must explicitly enable system.cpu.utilization
	// (off by default in OTel contrib v0.111+).
	hostmetrics := plan.AgentConfig.Receivers["hostmetrics"].(map[string]any)
	scrapers := hostmetrics["scrapers"].(map[string]any)
	cpuScraper := scrapers["cpu"].(map[string]any)
	cpuMetrics := cpuScraper["metrics"].(map[string]any)
	require.Contains(t, cpuMetrics, "system.cpu.utilization")

	// k8s_cluster always in gateway receivers.
	require.Contains(t, plan.GatewayConfig.Receivers, "k8s_cluster")

	// k8s_cluster must be in the gateway metrics pipeline receivers.
	gatewayMetricsPipeline := plan.GatewayConfig.Service.Pipelines["metrics"]
	require.Contains(t, gatewayMetricsPipeline.Receivers, "k8s_cluster")

	// k8s_cluster must explicitly enable container.ready, container.restarts,
	// and pod.status_reason (all default-disabled or recently default-flipped
	// in v0.111+; explicit enables make the contract visible in the plan).
	k8sCluster := plan.GatewayConfig.Receivers["k8s_cluster"].(map[string]any)
	enabledMetrics := k8sCluster["metrics"].(map[string]any)
	require.Contains(t, enabledMetrics, "k8s.container.ready")
	require.Contains(t, enabledMetrics, "k8s.container.restarts")
	require.Contains(t, enabledMetrics, "k8s.pod.status_reason")
}

// TestPlanSpanmetricsAndServicegraphConnectorsWhenOTLPTracesEnabled verifies
// that spanmetrics and servicegraph connectors are added when OTLP traces are
// enabled, and that the pipeline wiring follows the OTel connector pattern.
func TestPlanSpanmetricsAndServicegraphConnectorsWhenOTLPTracesEnabled(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources: []profile.ObservabilitySource{
			{
				Name:    "application-otlp",
				Type:    "otlp",
				Present: true,
				OTLP: &profile.OTLPSource{
					MetricsEnabled: false,
					TracesEnabled:  true,
					GRPCEndpoint:   "0.0.0.0:4317",
					HTTPEndpoint:   "0.0.0.0:4318",
				},
			},
		},
	})
	require.NoError(t, err)

	// Connectors must be present on the aggregation point (gateway), not the agent.
	require.Nil(t, plan.AgentConfig.Connectors)
	require.NotNil(t, plan.GatewayConfig.Connectors)
	require.Contains(t, plan.GatewayConfig.Connectors, "spanmetrics")
	require.Contains(t, plan.GatewayConfig.Connectors, "servicegraph")

	// Traces pipeline exporters must include the connectors on the gateway.
	tracesPipeline := plan.GatewayConfig.Service.Pipelines["traces"]
	require.Contains(t, tracesPipeline.Exporters, "spanmetrics")
	require.Contains(t, tracesPipeline.Exporters, "servicegraph")
	require.Contains(t, tracesPipeline.Exporters, "otlp")

	// Metrics pipeline receivers must include connector outputs on the gateway.
	metricsPipeline := plan.GatewayConfig.Service.Pipelines["metrics"]
	require.Contains(t, metricsPipeline.Receivers, "spanmetrics")
	require.Contains(t, metricsPipeline.Receivers, "servicegraph")

	// Agent traces should forward upstream only; connectors live on the gateway.
	agentTracesPipeline := plan.AgentConfig.Service.Pipelines["traces"]
	require.Equal(t, []string{"otlp"}, agentTracesPipeline.Exporters)
}

// TestPlanPrometheusAndTracesProfile verifies that when both prometheus and OTLP
// traces sources are present, the plan enables prometheus scraping, hostmetrics,
// spanmetrics/servicegraph connectors, and includes trace signal in EnabledSignals.
func TestPlanPrometheusAndTracesProfile(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources: []profile.ObservabilitySource{
			{
				Type:    "prometheus",
				Present: true,
				PrometheusTargets: []profile.PrometheusTarget{
					{ServiceName: "gateway-go", Namespace: "oksg-demo", Port: 8080, Path: "/metrics", Reachable: true},
				},
			},
			{
				Name:    "gateway-go-otlp",
				Type:    "otlp",
				Present: true,
				OTLP: &profile.OTLPSource{
					TracesEnabled: true,
					GRPCEndpoint:  "0.0.0.0:4317",
					HTTPEndpoint:  "0.0.0.0:4318",
				},
			},
		},
	})
	require.NoError(t, err)

	// Both signals enabled.
	require.Equal(t, []string{"metric", "trace"}, plan.EnabledSignals)

	// Prometheus scraping enabled.
	require.Contains(t, plan.AgentConfig.Receivers, "prometheus")

	// Connectors present for trace-derived metrics on the gateway.
	require.Nil(t, plan.AgentConfig.Connectors)
	require.Contains(t, plan.GatewayConfig.Connectors, "spanmetrics")
	require.Contains(t, plan.GatewayConfig.Connectors, "servicegraph")

	// Traces pipeline exports to both backend and connectors on the gateway.
	tracesPipeline := plan.GatewayConfig.Service.Pipelines["traces"]
	require.Contains(t, tracesPipeline.Exporters, "spanmetrics")
	require.Contains(t, tracesPipeline.Exporters, "servicegraph")
	require.Contains(t, tracesPipeline.Exporters, "otlp")

	// Agent metrics pipeline keeps its node-local receivers.
	agentMetricsPipeline := plan.AgentConfig.Service.Pipelines["metrics"]
	require.Contains(t, agentMetricsPipeline.Receivers, "prometheus")
	require.Contains(t, agentMetricsPipeline.Receivers, "kubeletstats")
	require.NotContains(t, agentMetricsPipeline.Receivers, "spanmetrics")
	require.NotContains(t, agentMetricsPipeline.Receivers, "servicegraph")

	agentTracesPipeline := plan.AgentConfig.Service.Pipelines["traces"]
	require.Equal(t, []string{"otlp"}, agentTracesPipeline.Exporters)

	// Gateway metrics pipeline receives from connectors alongside OTLP and k8s_cluster.
	metricsPipeline := plan.GatewayConfig.Service.Pipelines["metrics"]
	require.Contains(t, metricsPipeline.Receivers, "spanmetrics")
	require.Contains(t, metricsPipeline.Receivers, "servicegraph")
	require.Contains(t, metricsPipeline.Receivers, "otlp")
	require.Contains(t, metricsPipeline.Receivers, "k8s_cluster")
}

// TestPlanNoConnectorsWhenTracesNotEnabled verifies that connectors are absent
// when OTLP traces are not enabled, and the metrics pipeline does not include
// connector names.
func TestPlanNoConnectorsWhenTracesNotEnabled(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources:     []profile.ObservabilitySource{},
	})
	require.NoError(t, err)

	require.Nil(t, plan.AgentConfig.Connectors)
	require.Nil(t, plan.GatewayConfig.Connectors)

	metricsPipeline := plan.GatewayConfig.Service.Pipelines["metrics"]
	require.NotContains(t, metricsPipeline.Receivers, "spanmetrics")
	require.NotContains(t, metricsPipeline.Receivers, "servicegraph")
}

// TestPlanSuppressesServicegraphConnectorWhenIstioWins verifies that when
// istio is present together with OTLP traces, the planner does NOT deploy the
// servicegraph connector (istio wins all servicegraph.* metrics) but keeps
// spanmetrics (which has no overlapping winner).
func TestPlanSuppressesServicegraphConnectorWhenIstioWins(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources: []profile.ObservabilitySource{
			{
				Name:    "istio",
				Type:    "istio",
				Present: true,
				Mesh:    &profile.MeshSource{Flavor: "istio"},
			},
			{
				Name:    "application-otlp",
				Type:    "otlp",
				Present: true,
				OTLP:    &profile.OTLPSource{TracesEnabled: true},
			},
		},
	})
	require.NoError(t, err)

	require.Contains(t, plan.GatewayConfig.Connectors, "spanmetrics")
	require.NotContains(t, plan.GatewayConfig.Connectors, "servicegraph",
		"servicegraph connector must not deploy when istio wins all its metrics")

	tracesPipeline := plan.GatewayConfig.Service.Pipelines["traces"]
	require.Contains(t, tracesPipeline.Exporters, "spanmetrics")
	require.NotContains(t, tracesPipeline.Exporters, "servicegraph")

	metricsPipeline := plan.GatewayConfig.Service.Pipelines["metrics"]
	require.Contains(t, metricsPipeline.Receivers, "spanmetrics")
	require.NotContains(t, metricsPipeline.Receivers, "servicegraph")
}

// TestPlanInstallsOksgSourceTagger verifies the agent and gateway pipelines
// include the transform/oksg-source processor so every datapoint that lacks
// an explicit oksg_source attribute gets one.
func TestPlanInstallsOksgSourceTagger(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources:     []profile.ObservabilitySource{},
	})
	require.NoError(t, err)

	require.Contains(t, plan.AgentConfig.Processors, oksgSourceTaggerProcessor)
	require.Contains(t, plan.GatewayConfig.Processors, oksgSourceTaggerProcessor)

	require.Contains(t, plan.AgentConfig.Service.Pipelines["metrics"].Processors, oksgSourceTaggerProcessor)
	require.Contains(t, plan.GatewayConfig.Service.Pipelines["metrics"].Processors, oksgSourceTaggerProcessor)
}

func TestPlanTagsRawContainerMetricsAsKubelet(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources:     []profile.ObservabilitySource{},
	})
	require.NoError(t, err)

	tagger := plan.AgentConfig.Processors[oksgSourceTaggerProcessor].(map[string]any)
	metricStatements := tagger["metric_statements"].([]map[string]any)
	require.Len(t, metricStatements, 1)
	statements := metricStatements[0]["statements"].([]string)

	require.Contains(t, statements,
		`set(attributes["oksg_source"], "`+metrics.SourceKubelet+`") where IsMatch(metric.name, "^container\\.") and attributes["oksg_source"] == nil`,
	)
}

// TestPlanTagsK8sContainerMetricsAsKubeCluster verifies that container-scope
// metrics emitted by the k8s_cluster receiver (e.g. k8s.container.ready,
// k8s.container.restarts, k8s.container.cpu_request) are tagged kube-cluster,
// not kubelet. Mistagging these as kubelet causes the backend's
// shouldDropForSource to drop them (catalog assigns them to kube-cluster).
func TestPlanTagsK8sContainerMetricsAsKubeCluster(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources:     []profile.ObservabilitySource{},
	})
	require.NoError(t, err)

	tagger := plan.GatewayConfig.Processors[oksgSourceTaggerProcessor].(map[string]any)
	metricStatements := tagger["metric_statements"].([]map[string]any)
	require.Len(t, metricStatements, 1)
	statements := metricStatements[0]["statements"].([]string)

	require.Contains(t, statements,
		`set(attributes["oksg_source"], "`+metrics.SourceKubeCluster+`") where IsMatch(metric.name, "^k8s\\.container\\.") and attributes["oksg_source"] == nil`,
	)

	// Sanity: the old broad kubelet rule that conflated kubeletstats and
	// k8s_cluster pod/container metrics must be gone.
	require.NotContains(t, statements,
		`set(attributes["oksg_source"], "`+metrics.SourceKubelet+`") where IsMatch(metric.name, "^k8s\\.(pod|container|volume)\\.") and attributes["oksg_source"] == nil`,
	)
	require.NotContains(t, statements,
		`set(attributes["oksg_source"], "`+metrics.SourceKubelet+`") where IsMatch(metric.name, "^k8s\\.(pod|volume)\\.") and attributes["oksg_source"] == nil`,
	)
}

// TestPlanTagsKubeletNodeNetworkAndFilesystemAsKubelet verifies that
// k8s.node.network.* and k8s.node.filesystem.* (emitted by the kubeletstats
// receiver at v0.111+ when metric_groups includes "node") are tagged kubelet.
// Without this, the broader kube-cluster k8s.node.* allocatable/condition rule
// or no rule at all would let them flow through unlabeled and the backend
// shouldDropForSource would silently drop every datapoint.
func TestPlanTagsKubeletNodeNetworkAndFilesystemAsKubelet(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources:     []profile.ObservabilitySource{},
	})
	require.NoError(t, err)

	tagger := plan.AgentConfig.Processors[oksgSourceTaggerProcessor].(map[string]any)
	metricStatements := tagger["metric_statements"].([]map[string]any)
	require.Len(t, metricStatements, 1)
	statements := metricStatements[0]["statements"].([]string)

	require.Contains(t, statements,
		`set(attributes["oksg_source"], "`+metrics.SourceKubelet+`") where (metric.name == "k8s.node.cpu.usage" or metric.name == "k8s.node.memory.working_set" or IsMatch(metric.name, "^k8s\\.node\\.(network|filesystem)\\.")) and attributes["oksg_source"] == nil`,
	)
}

// TestPlanTagsK8sPodPhaseAsKubeCluster verifies that k8s.pod.phase (and any
// other pod-scope status signal from the k8s_cluster receiver) is tagged
// kube-cluster. The kubelet pod rule is tightened to match only the metric
// families kubeletstats actually emits at pod scope (cpu / memory /
// filesystem / network), so k8s.pod.phase falls through to the broad
// ^k8s\.pod\. kube-cluster rule below.
func TestPlanTagsK8sPodPhaseAsKubeCluster(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources:     []profile.ObservabilitySource{},
	})
	require.NoError(t, err)

	tagger := plan.GatewayConfig.Processors[oksgSourceTaggerProcessor].(map[string]any)
	metricStatements := tagger["metric_statements"].([]map[string]any)
	require.Len(t, metricStatements, 1)
	statements := metricStatements[0]["statements"].([]string)

	// Tightened kubelet pod rule (the only kubelet pod families on the wire).
	require.Contains(t, statements,
		`set(attributes["oksg_source"], "`+metrics.SourceKubelet+`") where IsMatch(metric.name, "^k8s\\.pod\\.(cpu|memory|filesystem|network)\\.") and attributes["oksg_source"] == nil`,
	)
	// Broad kube-cluster pod rule that catches phase/status_reason/etc.
	require.Contains(t, statements,
		`set(attributes["oksg_source"], "`+metrics.SourceKubeCluster+`") where IsMatch(metric.name, "^k8s\\.pod\\.") and attributes["oksg_source"] == nil`,
	)
}
