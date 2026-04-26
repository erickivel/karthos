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

	// k8s_cluster always in gateway receivers.
	require.Contains(t, plan.GatewayConfig.Receivers, "k8s_cluster")

	// k8s_cluster must be in the gateway metrics pipeline receivers.
	gatewayMetricsPipeline := plan.GatewayConfig.Service.Pipelines["metrics"]
	require.Contains(t, gatewayMetricsPipeline.Receivers, "k8s_cluster")
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
