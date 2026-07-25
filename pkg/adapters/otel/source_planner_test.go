package otel

import (
	"context"
	"regexp"
	"strings"
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
	require.Contains(t, plan.AgentConfig.Receivers, "kubelet_stats")
	require.NotContains(t, plan.AgentConfig.Receivers, "prometheus")
	require.Contains(t, plan.GatewayConfig.Receivers, "prometheus")
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

	prometheus := plan.GatewayConfig.Receivers["prometheus"].(map[string]any)
	config := prometheus["config"].(map[string]any)
	scrapeConfigs := config["scrape_configs"].([]map[string]any)
	require.Len(t, scrapeConfigs, 1)

	metricRelabels := scrapeConfigs[0]["metric_relabel_configs"].([]map[string]any)
	require.Contains(t, metricRelabels, map[string]any{
		"target_label": "oksg_source",
		"replacement":  metrics.SourcePrometheusApp,
	})
}

func TestPlanPrometheusAppScrapesServiceDNSAndPreservesTargetLabels(t *testing.T) {
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

	prometheus := plan.GatewayConfig.Receivers["prometheus"].(map[string]any)
	config := prometheus["config"].(map[string]any)
	scrapeConfigs := config["scrape_configs"].([]map[string]any)
	require.Len(t, scrapeConfigs, 1)

	job := scrapeConfigs[0]
	require.Equal(t, "/metrics", job["metrics_path"])
	require.NotContains(t, job, "kubernetes_sd_configs")
	require.NotContains(t, job, "relabel_configs")
	require.Equal(t, []map[string]any{
		{
			"targets": []string{"gateway-go.oksg-demo.svc.cluster.local:8080"},
			"labels": map[string]string{
				"service":      "gateway-go",
				"namespace":    "oksg-demo",
				"metrics_path": "/metrics",
				"service_port": "8080",
			},
		},
	}, job["static_configs"])
}

func TestPlanPrometheusAppJobNamesCannotCollideAcrossNamespaceAndService(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources: []profile.ObservabilitySource{
			{
				Type:    "prometheus",
				Present: true,
				PrometheusTargets: []profile.PrometheusTarget{
					{ServiceName: "c", Namespace: "a-b", Port: 8080, Path: "/metrics", Reachable: true},
					{ServiceName: "b-c", Namespace: "a", Port: 8080, Path: "/metrics", Reachable: true},
				},
			},
		},
	})
	require.NoError(t, err)

	prometheus := plan.GatewayConfig.Receivers["prometheus"].(map[string]any)
	config := prometheus["config"].(map[string]any)
	scrapeConfigs := config["scrape_configs"].([]map[string]any)
	require.Len(t, scrapeConfigs, 2)
	require.Equal(t, "prometheus-app:a-b:c:8080", scrapeConfigs[0]["job_name"])
	require.Equal(t, "prometheus-app:a:b-c:8080", scrapeConfigs[1]["job_name"])
	require.NotEqual(t, scrapeConfigs[0]["job_name"], scrapeConfigs[1]["job_name"])
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
	require.Equal(t, "15s", meshJob["scrape_interval"])
	require.Equal(t, []map[string]any{{"role": "pod"}}, meshJob["kubernetes_sd_configs"])
	require.NotContains(t, meshJob, "static_configs")

	relabelConfigs := meshJob["relabel_configs"].([]map[string]any)
	require.Contains(t, relabelConfigs, map[string]any{
		"source_labels": []string{"__meta_kubernetes_pod_container_name"},
		"action":        "keep",
		"regex":         "istio-proxy",
	})
	require.Contains(t, relabelConfigs, map[string]any{
		"source_labels": []string{"__meta_kubernetes_pod_node_name"},
		"action":        "keep",
		"regex":         `^\Q${env:K8S_NODE_NAME}\E$`,
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

func TestIstioNodeRelabelMatchesTheAgentNodeExactly(t *testing.T) {
	config := buildIstioScrapeConfig(&profile.MeshSource{Flavor: "istio"})
	relabelConfigs := config["relabel_configs"].([]map[string]any)

	var nodeRule map[string]any
	for _, rule := range relabelConfigs {
		if sourceLabels, ok := rule["source_labels"].([]string); ok && len(sourceLabels) == 1 && sourceLabels[0] == "__meta_kubernetes_pod_node_name" {
			nodeRule = rule
			break
		}
	}
	require.NotNil(t, nodeRule)

	resolved := strings.ReplaceAll(nodeRule["regex"].(string), "${env:K8S_NODE_NAME}", "node-1.example")
	matcher, err := regexp.Compile(resolved)
	require.NoError(t, err)
	require.True(t, matcher.MatchString("node-1.example"))
	require.False(t, matcher.MatchString("node-1xexample"))
}

func TestPlanUsesTraceServicegraphForPartialIstioMesh(t *testing.T) {
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
					InjectedNamespaces: []string{"oksg-demo"},
					Partial:            true,
				},
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

	require.NotContains(t, plan.AgentConfig.Receivers, "prometheus")
	require.Contains(t, plan.GatewayConfig.Connectors, "service_graph")
	require.Contains(t, plan.GatewayConfig.Service.Pipelines["traces"].Exporters, "service_graph")
	require.Contains(t, plan.GatewayConfig.Service.Pipelines["metrics"].Receivers, "service_graph")
}

func TestPlanOmitsServicegraphForPartialIstioMeshWithoutTraces(t *testing.T) {
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
					InjectedNamespaces: []string{"oksg-demo"},
					Partial:            true,
				},
			},
		},
	})
	require.NoError(t, err)

	require.NotContains(t, plan.AgentConfig.Receivers, "prometheus")
	require.NotContains(t, plan.GatewayConfig.Connectors, "service_graph")
	require.NotContains(t, plan.GatewayConfig.Service.Pipelines["metrics"].Receivers, "service_graph")
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
	require.Contains(t, plan.AgentConfig.Receivers, "kubelet_stats")
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
	require.Contains(t, plan.GatewayConfig.Exporters, "otlp_grpc")
	require.NotContains(t, plan.GatewayConfig.Exporters, "debug")
	require.Equal(t, []string{"metric", "trace"}, plan.EnabledSignals)
	require.Equal(t, DefaultBackendGRPCEndpoint, plan.Backend.GRPCEndpoint)
}

func TestPlanDoesNotInferSourceOnExternalOTLPMetricsPipelines(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")
	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterName: "kind-test",
		Sources: []profile.ObservabilitySource{{
			Type:    "otlp",
			Present: true,
			OTLP:    &profile.OTLPSource{MetricsEnabled: true},
		}},
	})
	require.NoError(t, err)

	agentExternal := plan.AgentConfig.Service.Pipelines["metrics/otlp"]
	require.Equal(t, []string{"otlp"}, agentExternal.Receivers)
	require.NotContains(t, agentExternal.Processors, oksgSourceTaggerProcessor)
	require.NotContains(t, plan.AgentConfig.Service.Pipelines["metrics"].Receivers, "otlp")

	gatewayExternal := plan.GatewayConfig.Service.Pipelines["metrics/otlp"]
	require.Equal(t, []string{"otlp"}, gatewayExternal.Receivers)
	require.NotContains(t, gatewayExternal.Processors, oksgSourceTaggerProcessor)
	require.NotContains(t, plan.GatewayConfig.Service.Pipelines["metrics"].Receivers, "otlp")

	// Internal collector-owned receivers retain source stamping at origin.
	require.Contains(t, plan.AgentConfig.Service.Pipelines["metrics"].Processors, oksgSourceTaggerProcessor)
	require.Contains(t, plan.GatewayConfig.Service.Pipelines["metrics"].Processors, oksgSourceTaggerProcessor)
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
	require.Contains(t, plan.GatewayConfig.Exporters, "otlp_grpc")
	require.Equal(t, DefaultBackendGRPCEndpoint, plan.Backend.GRPCEndpoint)
	require.Equal(t, []string{"metric"}, plan.EnabledSignals)
}

// TestPlanBaselineAlwaysIncludesHostmetricsAndKubeletstatsVolume verifies that
// host_metrics is always present in the agent receivers, kubelet_stats always
// includes "volume" in metric_groups, and k8s_cluster is always in the gateway.
func TestPlanBaselineAlwaysIncludesHostmetricsAndKubeletstatsVolume(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources:     []profile.ObservabilitySource{},
	})
	require.NoError(t, err)

	// host_metrics always in agent receivers.
	require.Contains(t, plan.AgentConfig.Receivers, "host_metrics")

	// kubelet_stats metric_groups must include "volume".
	kubelet_stats, ok := plan.AgentConfig.Receivers["kubelet_stats"].(map[string]any)
	require.True(t, ok, "kubelet_stats receiver must be map[string]any")
	groups, ok := kubelet_stats["metric_groups"].([]string)
	require.True(t, ok, "metric_groups must be []string")
	require.Contains(t, groups, "volume")

	// host_metrics must be in the agent metrics pipeline receivers.
	agentMetricsPipeline := plan.AgentConfig.Service.Pipelines["metrics"]
	require.Contains(t, agentMetricsPipeline.Receivers, "host_metrics")

	// host_metrics cpu scraper must explicitly enable system.cpu.utilization
	// (off by default in OTel contrib v0.156).
	host_metrics := plan.AgentConfig.Receivers["host_metrics"].(map[string]any)
	scrapers := host_metrics["scrapers"].(map[string]any)
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
	// in v0.156; explicit enables make the contract visible in the plan).
	k8sCluster := plan.GatewayConfig.Receivers["k8s_cluster"].(map[string]any)
	allocatableTypes := k8sCluster["allocatable_types_to_report"].([]string)
	require.Contains(t, allocatableTypes, "pods")
	enabledMetrics := k8sCluster["metrics"].(map[string]any)
	require.Contains(t, enabledMetrics, "k8s.container.ready")
	require.Contains(t, enabledMetrics, "k8s.container.restarts")
	require.Contains(t, enabledMetrics, "k8s.pod.status_reason")
}

func TestPlanUsesV0156ComponentNamesAndKubeletAccuracyOptions(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
	})
	require.NoError(t, err)

	require.Contains(t, plan.AgentConfig.Receivers, "kubelet_stats")
	require.Contains(t, plan.AgentConfig.Receivers, "host_metrics")
	require.Contains(t, plan.AgentConfig.Processors, "k8s_attributes")
	require.NotContains(t, plan.AgentConfig.Receivers, "kubeletstats")
	require.NotContains(t, plan.AgentConfig.Receivers, "hostmetrics")
	require.NotContains(t, plan.AgentConfig.Processors, "k8sattributes")

	kubelet := plan.AgentConfig.Receivers["kubelet_stats"].(map[string]any)
	require.Equal(t, map[string]any{"node": true, "pod": false}, kubelet["collect_all_network_interfaces"])
	require.Equal(t, []string{"k8s.volume.type"}, kubelet["extra_metadata_labels"])
	require.NotContains(t, kubelet, "k8s_api_config")

	metricsPipeline := plan.AgentConfig.Service.Pipelines["metrics"]
	require.Contains(t, metricsPipeline.Receivers, "kubelet_stats")
	require.Contains(t, metricsPipeline.Receivers, "host_metrics")
	require.Contains(t, metricsPipeline.Processors, "k8s_attributes")

	k8sCluster := plan.GatewayConfig.Receivers["k8s_cluster"].(map[string]any)
	metricConfig := k8sCluster["metrics"].(map[string]any)
	require.Equal(t, map[string]any{"enabled": false}, metricConfig["k8s.container.storage_limit"])
	require.Equal(t, map[string]any{"enabled": false}, metricConfig["k8s.container.storage_request"])
}

func TestPlanUsesV0156ConnectorNames(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")
	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources: []profile.ObservabilitySource{{
			Type:    "otlp",
			Present: true,
			OTLP: &profile.OTLPSource{
				TracesEnabled: true,
			},
		}},
	})
	require.NoError(t, err)

	require.Contains(t, plan.GatewayConfig.Connectors, "span_metrics")
	require.Contains(t, plan.GatewayConfig.Connectors, "service_graph")
	require.NotContains(t, plan.GatewayConfig.Connectors, "spanmetrics")
	require.NotContains(t, plan.GatewayConfig.Connectors, "servicegraph")
	require.Contains(t, plan.GatewayConfig.Service.Pipelines["metrics"].Receivers, "span_metrics")
	require.Contains(t, plan.GatewayConfig.Service.Pipelines["metrics"].Receivers, "service_graph")
	require.Contains(t, plan.GatewayConfig.Service.Pipelines["traces"].Exporters, "span_metrics")
	require.Contains(t, plan.GatewayConfig.Service.Pipelines["traces"].Exporters, "service_graph")
}

// TestPlanSpanmetricsAndServicegraphConnectorsWhenOTLPTracesEnabled verifies
// that span_metrics and service_graph connectors are added when OTLP traces are
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
	require.Contains(t, plan.GatewayConfig.Connectors, "span_metrics")
	require.Contains(t, plan.GatewayConfig.Connectors, "service_graph")
	service_graph := plan.GatewayConfig.Connectors["service_graph"].(map[string]any)
	require.Equal(t, "15s", service_graph["metrics_flush_interval"])
	store := service_graph["store"].(map[string]any)
	require.Equal(t, "30s", store["ttl"])

	// Traces pipeline exporters must include the connectors on the gateway.
	tracesPipeline := plan.GatewayConfig.Service.Pipelines["traces"]
	require.Contains(t, tracesPipeline.Exporters, "span_metrics")
	require.Contains(t, tracesPipeline.Exporters, "service_graph")
	require.Contains(t, tracesPipeline.Exporters, "otlp_grpc")

	// Metrics pipeline receivers must include connector outputs on the gateway.
	metricsPipeline := plan.GatewayConfig.Service.Pipelines["metrics"]
	require.Contains(t, metricsPipeline.Receivers, "span_metrics")
	require.Contains(t, metricsPipeline.Receivers, "service_graph")

	// Agent traces should forward upstream only; connectors live on the gateway.
	agentTracesPipeline := plan.AgentConfig.Service.Pipelines["traces"]
	require.Equal(t, []string{"otlp_grpc"}, agentTracesPipeline.Exporters)
}

// TestPlanPrometheusAndTracesProfile verifies that when both prometheus and OTLP
// traces sources are present, the plan enables prometheus scraping, host_metrics,
// span_metrics/service_graph connectors, and includes trace signal in EnabledSignals.
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

	// Application Prometheus scraping runs once on the singleton gateway.
	require.NotContains(t, plan.AgentConfig.Receivers, "prometheus")
	require.Contains(t, plan.GatewayConfig.Receivers, "prometheus")

	// Connectors present for trace-derived metrics on the gateway.
	require.Nil(t, plan.AgentConfig.Connectors)
	require.Contains(t, plan.GatewayConfig.Connectors, "span_metrics")
	require.Contains(t, plan.GatewayConfig.Connectors, "service_graph")

	// Traces pipeline exports to both backend and connectors on the gateway.
	tracesPipeline := plan.GatewayConfig.Service.Pipelines["traces"]
	require.Contains(t, tracesPipeline.Exporters, "span_metrics")
	require.Contains(t, tracesPipeline.Exporters, "service_graph")
	require.Contains(t, tracesPipeline.Exporters, "otlp_grpc")

	// Agent metrics pipeline keeps only its node-local receivers.
	agentMetricsPipeline := plan.AgentConfig.Service.Pipelines["metrics"]
	require.Contains(t, agentMetricsPipeline.Receivers, "kubelet_stats")
	require.NotContains(t, agentMetricsPipeline.Receivers, "prometheus")
	require.NotContains(t, agentMetricsPipeline.Receivers, "span_metrics")
	require.NotContains(t, agentMetricsPipeline.Receivers, "service_graph")

	agentTracesPipeline := plan.AgentConfig.Service.Pipelines["traces"]
	require.Equal(t, []string{"otlp_grpc"}, agentTracesPipeline.Exporters)

	// Gateway internal metrics pipeline receives singleton scrapes and connector output.
	metricsPipeline := plan.GatewayConfig.Service.Pipelines["metrics"]
	require.Contains(t, metricsPipeline.Receivers, "span_metrics")
	require.Contains(t, metricsPipeline.Receivers, "service_graph")
	require.Contains(t, metricsPipeline.Receivers, "prometheus")
	require.NotContains(t, metricsPipeline.Receivers, "otlp")
	require.Contains(t, metricsPipeline.Receivers, "k8s_cluster")
	require.Equal(t, []string{"otlp"}, plan.GatewayConfig.Service.Pipelines["metrics/otlp"].Receivers)
}

func TestPlanPrometheusAndTracesProfileWithControlPlaneOnlyIstio(t *testing.T) {
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
				OTLP:    &profile.OTLPSource{TracesEnabled: true},
			},
			{
				Name:    "istio",
				Type:    "istio",
				Present: true,
				Mesh:    &profile.MeshSource{Flavor: "istio", Mode: "sidecar"},
			},
		},
	})
	require.NoError(t, err)

	require.Equal(t, []string{"metric", "trace"}, plan.EnabledSignals)
	require.NotContains(t, plan.AgentConfig.Receivers, "prometheus")
	require.Contains(t, plan.GatewayConfig.Receivers, "prometheus")
	require.Contains(t, plan.GatewayConfig.Connectors, "span_metrics")
	require.Contains(t, plan.GatewayConfig.Connectors, "service_graph")

	tracesPipeline := plan.GatewayConfig.Service.Pipelines["traces"]
	require.Contains(t, tracesPipeline.Exporters, "span_metrics")
	require.Contains(t, tracesPipeline.Exporters, "service_graph")

	metricsPipeline := plan.GatewayConfig.Service.Pipelines["metrics"]
	require.Contains(t, metricsPipeline.Receivers, "span_metrics")
	require.Contains(t, metricsPipeline.Receivers, "service_graph")
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
	require.NotContains(t, metricsPipeline.Receivers, "span_metrics")
	require.NotContains(t, metricsPipeline.Receivers, "service_graph")
}

// TestPlanSuppressesServicegraphConnectorWhenIstioWins verifies that when
// istio is present together with OTLP traces, the planner does NOT deploy the
// service_graph connector (istio wins all service_graph.* metrics) but keeps
// span_metrics (which has no overlapping winner).
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
				Mesh:    &profile.MeshSource{Flavor: "istio", InjectedNamespaces: []string{"oksg-demo"}},
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

	require.Contains(t, plan.GatewayConfig.Connectors, "span_metrics")
	require.NotContains(t, plan.GatewayConfig.Connectors, "service_graph",
		"service_graph connector must not deploy when istio wins all its metrics")

	tracesPipeline := plan.GatewayConfig.Service.Pipelines["traces"]
	require.Contains(t, tracesPipeline.Exporters, "span_metrics")
	require.NotContains(t, tracesPipeline.Exporters, "service_graph")

	metricsPipeline := plan.GatewayConfig.Service.Pipelines["metrics"]
	require.Contains(t, metricsPipeline.Receivers, "span_metrics")
	require.NotContains(t, metricsPipeline.Receivers, "service_graph")
}

// TestPlanInstallsOksgSourceTagger verifies collector-owned receiver pipelines
// include transform/oksg-source. External OTLP metrics are intentionally
// isolated in metrics/otlp and covered separately above.
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

func TestPlanUsesOTLPGRPCExporterName(t *testing.T) {
	planner := NewSourcePlanner("oksg-system")

	plan, err := planner.Plan(context.Background(), &profile.ClusterProfile{
		ClusterID:   "abc123hash",
		ClusterName: "kind-test",
		Sources:     []profile.ObservabilitySource{},
	})
	require.NoError(t, err)

	for _, config := range []CollectorConfig{plan.AgentConfig, plan.GatewayConfig} {
		require.Contains(t, config.Exporters, "otlp_grpc")
		require.NotContains(t, config.Exporters, "otlp")
		for _, pipeline := range config.Service.Pipelines {
			require.NotContains(t, pipeline.Exporters, "otlp")
		}
	}
}

func TestOksgSourceTaggerUsesExplicitDatapointAttributes(t *testing.T) {
	tagger := buildOksgSourceTagger()
	metricStatements := tagger["metric_statements"].([]map[string]any)
	statements := metricStatements[0]["statements"].([]string)

	for _, statement := range statements {
		require.Contains(t, statement, `datapoint.attributes["oksg_source"]`)
		require.NotContains(t, statement, `set(attributes[`)
		require.NotContains(t, statement, ` and attributes[`)
	}
}

func TestOksgSourceTaggerRecognizesServicegraphConnectorWireNames(t *testing.T) {
	tagger := buildOksgSourceTagger()
	metricStatements := tagger["metric_statements"].([]map[string]any)
	statements := metricStatements[0]["statements"].([]string)

	require.Contains(t, statements,
		`set(datapoint.attributes["oksg_source"], "traces.servicegraph") where IsMatch(metric.name, "^traces_service_graph_") and datapoint.attributes["oksg_source"] == nil`)
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
		`set(datapoint.attributes["oksg_source"], "`+metrics.SourceKubelet+`") where IsMatch(metric.name, "^container\\.") and datapoint.attributes["oksg_source"] == nil`,
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
		`set(datapoint.attributes["oksg_source"], "`+metrics.SourceKubeCluster+`") where IsMatch(metric.name, "^k8s\\.container\\.") and datapoint.attributes["oksg_source"] == nil`,
	)

	// Sanity: the old broad kubelet rule that conflated kubelet_stats and
	// k8s_cluster pod/container metrics must be gone.
	require.NotContains(t, statements,
		`set(datapoint.attributes["oksg_source"], "`+metrics.SourceKubelet+`") where IsMatch(metric.name, "^k8s\\.(pod|container|volume)\\.") and datapoint.attributes["oksg_source"] == nil`,
	)
	require.NotContains(t, statements,
		`set(datapoint.attributes["oksg_source"], "`+metrics.SourceKubelet+`") where IsMatch(metric.name, "^k8s\\.(pod|volume)\\.") and datapoint.attributes["oksg_source"] == nil`,
	)
}

// TestPlanTagsKubeletNodeNetworkAndFilesystemAsKubelet verifies that
// k8s.node.network.* and k8s.node.filesystem.* (emitted by the kubelet_stats
// receiver at v0.156 when metric_groups includes "node") are tagged kubelet.
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
		`set(datapoint.attributes["oksg_source"], "`+metrics.SourceKubelet+`") where (metric.name == "k8s.node.cpu.usage" or metric.name == "k8s.node.memory.working_set" or IsMatch(metric.name, "^k8s\\.node\\.(network|filesystem)\\.")) and datapoint.attributes["oksg_source"] == nil`,
	)
}

// TestPlanTagsK8sPodPhaseAsKubeCluster verifies that k8s.pod.phase (and any
// other pod-scope status signal from the k8s_cluster receiver) is tagged
// kube-cluster. The kubelet pod rule is tightened to match only the metric
// families kubelet_stats actually emits at pod scope (cpu / memory /
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
		`set(datapoint.attributes["oksg_source"], "`+metrics.SourceKubelet+`") where IsMatch(metric.name, "^k8s\\.pod\\.(cpu|memory|filesystem|network)\\.") and datapoint.attributes["oksg_source"] == nil`,
	)
	// Broad kube-cluster pod rule that catches phase/status_reason/etc.
	require.Contains(t, statements,
		`set(datapoint.attributes["oksg_source"], "`+metrics.SourceKubeCluster+`") where IsMatch(metric.name, "^k8s\\.pod\\.") and datapoint.attributes["oksg_source"] == nil`,
	)
}

func TestPlanTagsKubeClusterWorkloadFamiliesAsKubeCluster(t *testing.T) {
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
		`set(datapoint.attributes["oksg_source"], "`+metrics.SourceKubeCluster+`") where IsMatch(metric.name, "^k8s\\.(replicaset|deployment|namespace|daemonset|statefulset|job|cronjob|hpa|resource_quota|replication_controller)\\.") and datapoint.attributes["oksg_source"] == nil`,
	)
}
