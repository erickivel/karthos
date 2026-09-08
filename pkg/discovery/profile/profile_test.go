package profile

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClusterProfileSerializesOnlyModuleCapabilities(t *testing.T) {
	profile := ClusterProfile{
		Capabilities: []SourceCapability{
			{
				SourceID: "prometheus.app",
				Kind:     SourceKindDirect,
				Status:   CapabilityAvailable,
				Signals:  []string{"metric"},
				Evidence: CapabilityEvidence{PrometheusTargets: []PrometheusTarget{{
					ServiceName: "gateway-go",
					Namespace:   "oksg-demo",
					Port:        8080,
					Path:        "/metrics",
				}},
				},
			},
		},
	}

	payload, err := json.Marshal(profile)
	require.NoError(t, err)

	require.Contains(t, string(payload), "capabilities")
	require.Contains(t, string(payload), "prometheus.app")
}

func TestClusterProfileIncludesDerivedServicegraphEvidence(t *testing.T) {
	profile := ClusterProfile{
		Capabilities: []SourceCapability{
			{
				SourceID: "traces.servicegraph",
				Kind:     SourceKindDerived,
				Status:   CapabilityAvailable,
				Signals:  []string{"metric"},
				Evidence: CapabilityEvidence{OTLP: &OTLPEvidence{
					TracesEnabled: true,
				}},
			},
		},
	}

	payload, err := json.Marshal(profile)
	require.NoError(t, err)

	require.Contains(t, string(payload), "traces.servicegraph")
	require.Contains(t, string(payload), "traces_enabled")
}

func TestFingerprintIsStableAcrossEvidenceOrdering(t *testing.T) {
	first := &ClusterProfile{ClusterName: "demo", Capabilities: []SourceCapability{
		{SourceID: "prometheus.app", Kind: SourceKindDirect, Status: CapabilityAvailable, Signals: []string{"metric"}, Evidence: CapabilityEvidence{PrometheusTargets: []PrometheusTarget{
			{ServiceName: "zeta", Namespace: "demo", Port: 8080, Path: "/metrics"},
			{ServiceName: "alpha", Namespace: "demo", Port: 8080, Path: "/metrics"},
		}}},
		{SourceID: "mesh.istio", Kind: SourceKindDirect, Status: CapabilityAvailable, Signals: []string{"metric"}, Evidence: CapabilityEvidence{Mesh: &MeshEvidence{Flavor: "istio", Mode: "sidecar", InjectedNamespaces: []string{"team-b", "team-a"}, MetricsPort: 15020, MetricsPath: "/stats/prometheus"}}},
	}}
	second := &ClusterProfile{ClusterName: "demo", Capabilities: []SourceCapability{
		{SourceID: "mesh.istio", Kind: SourceKindDirect, Status: CapabilityAvailable, Signals: []string{"metric"}, Evidence: CapabilityEvidence{Mesh: &MeshEvidence{Flavor: "istio", Mode: "sidecar", InjectedNamespaces: []string{"team-a", "team-b"}, MetricsPort: 15020, MetricsPath: "/stats/prometheus"}}},
		{SourceID: "prometheus.app", Kind: SourceKindDirect, Status: CapabilityAvailable, Signals: []string{"metric"}, Evidence: CapabilityEvidence{PrometheusTargets: []PrometheusTarget{
			{ServiceName: "alpha", Namespace: "demo", Port: 8080, Path: "/metrics"},
			{ServiceName: "zeta", Namespace: "demo", Port: 8080, Path: "/metrics"},
		}}},
	}}

	firstFingerprint, err := first.Fingerprint()
	require.NoError(t, err)
	secondFingerprint, err := second.Fingerprint()
	require.NoError(t, err)
	require.Equal(t, firstFingerprint, secondFingerprint)
}

func TestFingerprintIsStableAcrossCapabilityOrdering(t *testing.T) {
	first := &ClusterProfile{ClusterName: "demo", Capabilities: []SourceCapability{
		{SourceID: "prometheus.app", Kind: SourceKindDirect, Status: CapabilityUnavailable},
		{SourceID: "kubelet", Kind: SourceKindDirect, Status: CapabilityAvailable, Signals: []string{"metric"}},
	}}
	second := &ClusterProfile{ClusterName: "demo", Capabilities: []SourceCapability{
		{SourceID: "kubelet", Kind: SourceKindDirect, Status: CapabilityAvailable, Signals: []string{"metric"}},
		{SourceID: "prometheus.app", Kind: SourceKindDirect, Status: CapabilityUnavailable},
	}}

	firstFingerprint, err := first.Fingerprint()
	require.NoError(t, err)
	secondFingerprint, err := second.Fingerprint()
	require.NoError(t, err)
	require.Equal(t, firstFingerprint, secondFingerprint)
}
