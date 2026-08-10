package otel

import (
	"context"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

func TestPlanUsesOnlySelectedModuleContributions(t *testing.T) {
	plan, err := NewSourcePlanner("oksg-system").Plan(context.Background(), fullCapabilityProfile())
	require.NoError(t, err)

	require.Contains(t, plan.GatewayConfig.Receivers, "prometheus")
	require.Contains(t, plan.AgentConfig.Receivers, "prometheus")
	// Istio wins the shared servicegraph metrics under catalog priority, so the
	// trace-derived connector is not deployed merely because traces are present.
	require.NotContains(t, plan.GatewayConfig.Connectors, "service_graph")
	require.Contains(t, plan.AgentConfig.Service.Pipelines, "traces")
	require.NotEmpty(t, plan.SourcePlan.ProfileFingerprint)
	require.Equal(t, metrics.SourceMeshIstio, plan.SourcePlan.MetricAssignments["servicegraph.request.count"])
}

func TestPlanExcludesUnavailableOptionalModules(t *testing.T) {
	profile := baselineCapabilityProfile()
	plan, err := NewSourcePlanner("oksg-system").Plan(context.Background(), profile)
	require.NoError(t, err)

	require.NotContains(t, plan.GatewayConfig.Receivers, "prometheus")
	require.NotContains(t, plan.AgentConfig.Receivers, "prometheus")
	require.Empty(t, plan.GatewayConfig.Connectors)
	require.NotContains(t, plan.AgentConfig.Service.Pipelines, "traces")
}

func TestPlanRejectsIncompleteCompiledModuleInventory(t *testing.T) {
	p := baselineCapabilityProfile()
	p.Capabilities = p.Capabilities[1:]
	_, err := NewSourcePlanner("oksg-system").Plan(context.Background(), p)
	require.ErrorContains(t, err, "capability profile")
}

func baselineCapabilityProfile() *profile.ClusterProfile {
	return &profile.ClusterProfile{ClusterName: "kind-test", Capabilities: []profile.SourceCapability{
		{SourceID: metrics.SourceKubernetesStructure, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"structure"}},
		{SourceID: metrics.SourceKubelet, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
		{SourceID: metrics.SourceHostOS, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
		{SourceID: metrics.SourceKubeCluster, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
		{SourceID: metrics.SourcePrometheusApp, Kind: profile.SourceKindDirect, Status: profile.CapabilityUnavailable},
		{SourceID: metrics.SourceMeshIstio, Kind: profile.SourceKindDirect, Status: profile.CapabilityUnavailable},
		{SourceID: metrics.SourceTracesServicegraph, Kind: profile.SourceKindDerived, Status: profile.CapabilityUnavailable},
	}}
}

func fullCapabilityProfile() *profile.ClusterProfile {
	p := baselineCapabilityProfile()
	p.Capabilities[4] = profile.SourceCapability{
		SourceID: metrics.SourcePrometheusApp, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"},
		Evidence: profile.CapabilityEvidence{PrometheusTargets: []profile.PrometheusTarget{{ServiceName: "gateway", Namespace: "demo", Port: 8080, Path: "/metrics"}}},
	}
	p.Capabilities[5] = profile.SourceCapability{
		SourceID: metrics.SourceMeshIstio, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"},
		Evidence: profile.CapabilityEvidence{Mesh: &profile.MeshEvidence{Flavor: "istio", InjectedNamespaces: []string{"demo"}}},
	}
	p.Capabilities[6] = profile.SourceCapability{
		SourceID: metrics.SourceTracesServicegraph, Kind: profile.SourceKindDerived, Status: profile.CapabilityAvailable, Signals: []string{"metric"},
		Evidence: profile.CapabilityEvidence{OTLP: &profile.OTLPEvidence{TracesEnabled: true}},
	}
	return p
}
