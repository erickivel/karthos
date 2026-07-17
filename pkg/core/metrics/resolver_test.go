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

	assignment, selected, err := Resolve(catalog, DefaultLogicalSources(), emptyProfile())
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

	assignment, selected, err := Resolve(catalog, DefaultLogicalSources(), emptyProfile())
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

	assignment, selected, err := Resolve(catalog, DefaultLogicalSources(), prof)
	require.NoError(t, err)
	require.Equal(t, SourceMeshIstio, assignment["servicegraph.request.count"])
	require.True(t, selected.Has(SourceMeshIstio))
	require.False(t, selected.Has(SourceTracesServicegraph), "traces.servicegraph should not be selected when istio wins all its metrics")
}

func TestResolveTracesServicegraphWinsWhenIstioAbsent(t *testing.T) {
	catalog := []CanonicalMetric{
		{Name: "servicegraph.request.count", Sources: []SourceRef{
			{ID: SourceMeshIstio, Priority: 1},
			{ID: SourceTracesServicegraph, Priority: 2},
		}},
	}

	prof := &profile.ClusterProfile{
		Sources: []profile.ObservabilitySource{
			{Type: "otlp", Present: true, OTLP: &profile.OTLPSource{TracesEnabled: true}},
		},
	}

	assignment, selected, err := Resolve(catalog, DefaultLogicalSources(), prof)
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
		Sources: []profile.ObservabilitySource{
			{
				Type:    "istio",
				Present: true,
				Mesh:    &profile.MeshSource{Flavor: "istio", Mode: "sidecar"},
			},
			{Type: "otlp", Present: true, OTLP: &profile.OTLPSource{TracesEnabled: true}},
		},
	}

	assignment, selected, err := Resolve(catalog, DefaultLogicalSources(), prof)
	require.NoError(t, err)
	require.Equal(t, SourceTracesServicegraph, assignment["servicegraph.request.count"])
	require.False(t, selected.Has(SourceMeshIstio))
	require.True(t, selected.Has(SourceTracesServicegraph))
}

func TestResolveErrorsOnUnknownSource(t *testing.T) {
	catalog := []CanonicalMetric{
		{Name: "made.up", Sources: []SourceRef{{ID: "nope", Priority: 1}}},
	}

	_, _, err := Resolve(catalog, DefaultLogicalSources(), emptyProfile())
	require.ErrorContains(t, err, "unknown source")
}

func TestResolveErrorsOnUnimplementedPolicy(t *testing.T) {
	catalog := []CanonicalMetric{
		{
			Name:            "servicegraph.request.count",
			Sources:         []SourceRef{{ID: SourceMeshIstio, Priority: 1}},
			SelectionPolicy: PolicyDisjoint,
		},
	}

	_, _, err := Resolve(catalog, DefaultLogicalSources(), profileWithIstioAndTraces())
	require.ErrorContains(t, err, "unimplemented selection policy")
}

func TestResolveDefaultCatalogAgainstFullProfile(t *testing.T) {
	prof := profileWithIstioAndTraces()
	prof.Sources = append(prof.Sources, profile.ObservabilitySource{
		Type:    "prometheus",
		Present: true,
		PrometheusTargets: []profile.PrometheusTarget{
			{ServiceName: "demo", Namespace: "demo", Port: 8080, Path: "/metrics"},
		},
	})

	assignment, selected, err := Resolve(DefaultCatalog(), DefaultLogicalSources(), prof)
	require.NoError(t, err)

	require.Equal(t, SourceMeshIstio, assignment["servicegraph.request.count"])
	require.Equal(t, SourceKubelet, assignment["k8s.pod.cpu.usage"])
	require.Equal(t, SourcePrometheusApp, assignment["http.server.request.count"])
	require.Equal(t, SourceTracesSpanmetrics, assignment["rpc.server.duration"])

	require.True(t, selected.Has(SourceMeshIstio))
	require.True(t, selected.Has(SourceTracesSpanmetrics))
	require.True(t, selected.Has(SourcePrometheusApp))
	// traces.servicegraph wins zero metrics because mesh.istio outranks it everywhere.
	require.False(t, selected.Has(SourceTracesServicegraph))
}

func emptyProfile() *profile.ClusterProfile { return &profile.ClusterProfile{} }

func profileWithIstioAndTraces() *profile.ClusterProfile {
	return &profile.ClusterProfile{
		Sources: []profile.ObservabilitySource{
			{Type: "istio", Present: true, Mesh: &profile.MeshSource{Flavor: "istio", InjectedNamespaces: []string{"oksg-demo"}}},
			{Type: "otlp", Present: true, OTLP: &profile.OTLPSource{TracesEnabled: true}},
		},
	}
}
