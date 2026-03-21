package otel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

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

	// Connectors must be present.
	require.NotNil(t, plan.AgentConfig.Connectors)
	require.Contains(t, plan.AgentConfig.Connectors, "spanmetrics")
	require.Contains(t, plan.AgentConfig.Connectors, "servicegraph")

	// Traces pipeline exporters must include the connectors.
	tracesPipeline := plan.AgentConfig.Service.Pipelines["traces"]
	require.Contains(t, tracesPipeline.Exporters, "spanmetrics")
	require.Contains(t, tracesPipeline.Exporters, "servicegraph")
	require.Contains(t, tracesPipeline.Exporters, "otlp")

	// Metrics pipeline receivers must include connector outputs.
	metricsPipeline := plan.AgentConfig.Service.Pipelines["metrics"]
	require.Contains(t, metricsPipeline.Receivers, "spanmetrics")
	require.Contains(t, metricsPipeline.Receivers, "servicegraph")
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

	// Connectors present for trace-derived metrics.
	require.Contains(t, plan.AgentConfig.Connectors, "spanmetrics")
	require.Contains(t, plan.AgentConfig.Connectors, "servicegraph")

	// Traces pipeline exports to both backend and connectors.
	tracesPipeline := plan.AgentConfig.Service.Pipelines["traces"]
	require.Contains(t, tracesPipeline.Exporters, "spanmetrics")
	require.Contains(t, tracesPipeline.Exporters, "servicegraph")
	require.Contains(t, tracesPipeline.Exporters, "otlp")

	// Metrics pipeline receives from connectors alongside prometheus and kubeletstats.
	metricsPipeline := plan.AgentConfig.Service.Pipelines["metrics"]
	require.Contains(t, metricsPipeline.Receivers, "spanmetrics")
	require.Contains(t, metricsPipeline.Receivers, "servicegraph")
	require.Contains(t, metricsPipeline.Receivers, "prometheus")
	require.Contains(t, metricsPipeline.Receivers, "kubeletstats")
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

	metricsPipeline := plan.AgentConfig.Service.Pipelines["metrics"]
	require.NotContains(t, metricsPipeline.Receivers, "spanmetrics")
	require.NotContains(t, metricsPipeline.Receivers, "servicegraph")
}
