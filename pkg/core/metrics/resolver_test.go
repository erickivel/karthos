package metrics

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/discovery/profile"
)

func TestResolveSingleSourcePicksOnlyAvailable(t *testing.T) {
	catalog := []CanonicalMetric{
		{Name: "k8s.pod.cpu.usage", Sources: []SourceRef{{ID: SourceKubelet, Priority: 1}}},
	}

	assignment, selected, err := resolveWithProfile(catalog, emptyProfile())
	require.NoError(t, err)
	require.Equal(t, SourceKubelet, assignment["k8s.pod.cpu.usage"])
	require.True(t, selected.Has(SourceKubelet))
}

func TestResolveDropsMetricWhenNoSourceAvailable(t *testing.T) {
	catalog := []CanonicalMetric{
		{Name: "servicegraph.request.count", Sources: []SourceRef{
			{ID: SourceMeshIstio, Priority: 1},
			{ID: SourceTracesServicegraph, Priority: 2},
		}},
	}

	assignment, selected, err := resolveWithProfile(catalog, emptyProfile())
	require.NoError(t, err)
	_, ok := assignment["servicegraph.request.count"]
	require.False(t, ok, "metric should be omitted when no source is available")
	require.False(t, selected.Has(SourceMeshIstio))
	require.False(t, selected.Has(SourceTracesServicegraph))
}

func TestResolveIstioWinsOverTracesServicegraph(t *testing.T) {
	catalog := []CanonicalMetric{
		{Name: "servicegraph.request.count", Sources: []SourceRef{
			{ID: SourceMeshIstio, Priority: 1},
			{ID: SourceTracesServicegraph, Priority: 2},
		}},
	}

	prof := profileWithIstioAndTraces()

	assignment, selected, err := resolveWithProfile(catalog, prof)
	require.NoError(t, err)
	require.Equal(t, SourceMeshIstio, assignment["servicegraph.request.count"])
	require.True(t, selected.Has(SourceMeshIstio))
	require.False(t, selected.Has(SourceTracesServicegraph), "traces.servicegraph should not be selected when istio wins all its metrics")
}

func TestResolveTracesServicegraphWinsWhenIstioMeshIsPartial(t *testing.T) {
	catalog := []CanonicalMetric{
		{Name: "servicegraph.request.count", Sources: []SourceRef{
			{ID: SourceMeshIstio, Priority: 1},
			{ID: SourceTracesServicegraph, Priority: 2},
		}},
	}

	prof := profileWithIstioAndTraces()
	prof.Capabilities[3].Status = profile.CapabilityPartial

	assignment, selected, err := resolveWithProfile(catalog, prof)
	require.NoError(t, err)
	require.Equal(t, SourceTracesServicegraph, assignment["servicegraph.request.count"])
	require.False(t, selected.Has(SourceMeshIstio))
	require.True(t, selected.Has(SourceTracesServicegraph))
}

func TestResolveTracesServicegraphWinsWhenIstioAbsent(t *testing.T) {
	catalog := []CanonicalMetric{
		{Name: "servicegraph.request.count", Sources: []SourceRef{
			{ID: SourceMeshIstio, Priority: 1},
			{ID: SourceTracesServicegraph, Priority: 2},
		}},
	}

	prof := &profile.ClusterProfile{
		Capabilities: []profile.SourceCapability{
			{SourceID: SourceTracesServicegraph, Kind: profile.SourceKindDerived, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
		},
	}

	assignment, selected, err := resolveWithProfile(catalog, prof)
	require.NoError(t, err)
	require.Equal(t, SourceTracesServicegraph, assignment["servicegraph.request.count"])
	require.True(t, selected.Has(SourceTracesServicegraph))
}

func TestResolveTracesServicegraphWinsWhenIstioHasNoInjectedNamespaces(t *testing.T) {
	catalog := []CanonicalMetric{
		{Name: "servicegraph.request.count", Sources: []SourceRef{
			{ID: SourceMeshIstio, Priority: 1},
			{ID: SourceTracesServicegraph, Priority: 2},
		}},
	}

	prof := &profile.ClusterProfile{
		Capabilities: []profile.SourceCapability{
			{SourceID: SourceMeshIstio, Kind: profile.SourceKindDirect, Status: profile.CapabilityPartial, Signals: []string{"metric"}},
			{SourceID: SourceTracesServicegraph, Kind: profile.SourceKindDerived, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
		},
	}

	assignment, selected, err := resolveWithProfile(catalog, prof)
	require.NoError(t, err)
	require.Equal(t, SourceTracesServicegraph, assignment["servicegraph.request.count"])
	require.False(t, selected.Has(SourceMeshIstio))
	require.True(t, selected.Has(SourceTracesServicegraph))
}

func TestResolveErrorsOnUnknownSource(t *testing.T) {
	catalog := []CanonicalMetric{
		{Name: "made.up", Sources: []SourceRef{{ID: "nope", Priority: 1}}},
	}

	_, _, err := resolveWithProfile(catalog, emptyProfile())
	require.ErrorContains(t, err, "unknown source")
}

func TestResolveDefaultCatalogAgainstFullProfile(t *testing.T) {
	prof := profileWithIstioAndTraces()
	prof.Capabilities = append(prof.Capabilities, profile.SourceCapability{
		SourceID: SourcePrometheusApp, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"},
	})

	assignment, selected, err := resolveWithProfile(DefaultCatalog(), prof)
	require.NoError(t, err)

	require.Equal(t, SourceMeshIstio, assignment["servicegraph.request.count"])
	require.Equal(t, SourceKubelet, assignment["k8s.pod.cpu.usage"])
	require.Equal(t, SourcePrometheusApp, assignment["http.server.request.count"])
	require.True(t, selected.Has(SourceMeshIstio))
	require.True(t, selected.Has(SourcePrometheusApp))
	// traces.servicegraph wins zero metrics because mesh.istio outranks it everywhere.
	require.False(t, selected.Has(SourceTracesServicegraph))
}

func emptyProfile() *profile.ClusterProfile {
	return &profile.ClusterProfile{Capabilities: []profile.SourceCapability{
		{SourceID: SourceKubelet, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
		{SourceID: SourceHostOS, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
		{SourceID: SourceKubeCluster, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
	}}
}

func profileWithIstioAndTraces() *profile.ClusterProfile {
	return &profile.ClusterProfile{
		Capabilities: []profile.SourceCapability{
			{SourceID: SourceKubelet, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
			{SourceID: SourceHostOS, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
			{SourceID: SourceKubeCluster, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
			{SourceID: SourceMeshIstio, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
			{SourceID: SourceTracesServicegraph, Kind: profile.SourceKindDerived, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
		},
	}
}

func resolveWithProfile(catalog []CanonicalMetric, capabilityProfile *profile.ClusterProfile) (Assignment, SelectedSources, error) {
	return Resolve(catalog, []string{
		SourceKubelet, SourceHostOS, SourceKubeCluster, SourcePrometheusApp,
		SourceMeshIstio, SourceTracesServicegraph,
	}, capabilityProfile.IsAvailable)
}
