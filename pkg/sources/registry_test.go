package sources

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDefaultRegistryContainsEveryCompiledModuleExactlyOnce(t *testing.T) {
	registry := DefaultRegistry()
	require.Equal(t, []string{
		IDKubernetesStructure,
		IDKubelet,
		IDHostOS,
		IDKubeCluster,
		IDPrometheusApp,
		IDMeshIstio,
		IDTracesServicegraph,
	}, registry.IDs())
	structure, ok := registry.Lookup(IDKubernetesStructure)
	require.True(t, ok)
	require.False(t, structure.MetricSource)
	prometheus, ok := registry.Lookup(IDPrometheusApp)
	require.True(t, ok)
	require.True(t, prometheus.MetricSource)
}
