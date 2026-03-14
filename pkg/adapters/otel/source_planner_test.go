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
