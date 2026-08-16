package kubecluster

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/core/metrics"
)

func TestDialectTranslatesKubeClusterContainerSpecs(t *testing.T) {
	for raw, canonical := range map[string]string{
		"k8s.container.cpu_request":    "k8s.container.cpu.request",
		"k8s.container.cpu_limit":      "k8s.container.cpu.limit",
		"k8s.container.memory_request": "k8s.container.memory.request",
		"k8s.container.memory_limit":   "k8s.container.memory.limit",
	} {
		got, ok := Dialect.Translate(raw)
		require.True(t, ok)
		require.Equal(t, canonical, got)
	}
}

func TestDialectCoversCatalogKubeClusterMetrics(t *testing.T) {
	passthroughs := map[string]struct{}{
		"k8s.container.ready":                {},
		"k8s.container.restarts":             {},
		"k8s.pod.phase":                      {},
		"k8s.pod.status_reason":              {},
		"k8s.node.condition.ready":           {},
		"k8s.node.condition.memory_pressure": {},
		"k8s.node.condition.disk_pressure":   {},
	}
	canonicals := make(map[string]struct{}, len(Dialect))
	for _, canonical := range Dialect {
		canonicals[canonical] = struct{}{}
	}
	for _, metric := range metrics.DefaultCatalog() {
		hasKubeCluster := false
		for _, source := range metric.Sources {
			if source.ID == metrics.SourceKubeCluster {
				hasKubeCluster = true
				break
			}
		}
		if !hasKubeCluster {
			continue
		}
		_, translated := canonicals[metric.Name]
		_, passthrough := passthroughs[metric.Name]
		require.Truef(t, translated || passthrough, "catalog metric %q needs a dialect mapping or supported passthrough", metric.Name)
	}
}
