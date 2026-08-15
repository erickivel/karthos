package collector

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources"
)

func TestComposeKeepsTraceIntakeWhenIstioOwnsServiceGraphMetrics(t *testing.T) {
	capabilityProfile := completeProfile()
	registry := sources.DefaultRegistry()
	sourcePlan, err := registry.ResolveSourcePlan(capabilityProfile)
	require.NoError(t, err)
	require.Equal(t, metrics.SourceMeshIstio, sourcePlan.MetricAssignments["servicegraph.request.count"])

	plan, err := NewComposer("oksg-system").Compose(capabilityProfile, sourcePlan)
	require.NoError(t, err)
	require.True(t, plan.Agent.OTLP.Traces)
	require.True(t, plan.Gateway.OTLP.Traces)
	require.Nil(t, plan.Gateway.ServiceGraph)
}

func TestComposeAddsDerivedServiceGraphOnlyWhenSelected(t *testing.T) {
	capabilityProfile := completeProfile()
	capabilityProfile.Capabilities[5].Status = profile.CapabilityUnavailable
	registry := sources.DefaultRegistry()
	sourcePlan, err := registry.ResolveSourcePlan(capabilityProfile)
	require.NoError(t, err)
	require.Equal(t, metrics.SourceTracesServicegraph, sourcePlan.MetricAssignments["servicegraph.request.count"])

	plan, err := NewComposer("oksg-system").Compose(capabilityProfile, sourcePlan)
	require.NoError(t, err)
	require.NotNil(t, plan.Gateway.ServiceGraph)
	require.Equal(t, metrics.SourceTracesServicegraph, plan.Gateway.ServiceGraph.SourceID)
}

func completeProfile() *profile.ClusterProfile {
	return &profile.ClusterProfile{ClusterName: "kind-test", Capabilities: []profile.SourceCapability{
		{SourceID: metrics.SourceKubernetesStructure, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"structure"}},
		{SourceID: metrics.SourceKubelet, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
		{SourceID: metrics.SourceHostOS, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
		{SourceID: metrics.SourceKubeCluster, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
		{SourceID: metrics.SourcePrometheusApp, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}, Evidence: profile.CapabilityEvidence{PrometheusTargets: []profile.PrometheusTarget{{ServiceName: "gateway", Namespace: "demo", Port: 8080, Path: "/metrics"}}}},
		{SourceID: metrics.SourceMeshIstio, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}, Evidence: profile.CapabilityEvidence{Mesh: &profile.MeshEvidence{Flavor: "istio", InjectedNamespaces: []string{"demo"}}}},
		{SourceID: metrics.SourceTracesServicegraph, Kind: profile.SourceKindDerived, Status: profile.CapabilityAvailable, Signals: []string{"metric"}, Evidence: profile.CapabilityEvidence{OTLP: &profile.OTLPEvidence{TracesEnabled: true}}},
	}}
}
