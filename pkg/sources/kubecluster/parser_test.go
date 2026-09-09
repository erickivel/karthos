package kubecluster

import (
	"testing"

	"github.com/stretchr/testify/require"

	collectorspec "github.com/erickivel/karthos/pkg/collector/spec"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/discovery/profile"
)

func TestContributeStampsRawNodeCondition(t *testing.T) {
	plan := collectorspec.CollectionPlan{}
	Contribute(&plan, profile.SourceCapability{}, true)

	require.Len(t, plan.Gateway.SourceStamps, 1)
	stamp := plan.Gateway.SourceStamps[0]
	require.Equal(t, metrics.SourceKubeCluster, stamp.SourceID)
	require.Contains(t, stamp.ExactMetricNames, "k8s.node.condition")
	require.True(t, stamp.OnlyIfUnset)
}

func TestParseTranslatesKubeClusterContainerSpecs(t *testing.T) {
	for raw, canonical := range map[string]string{
		"k8s.container.cpu_request":    "k8s.container.cpu.request",
		"k8s.container.cpu_limit":      "k8s.container.cpu.limit",
		"k8s.container.memory_request": "k8s.container.memory.request",
		"k8s.container.memory_limit":   "k8s.container.memory.limit",
	} {
		parsed, ok := Parse(raw, nil)
		require.True(t, ok)
		require.Equal(t, canonical, parsed.CanonicalName)
	}
}

func TestParseCoversCatalogKubeClusterMetrics(t *testing.T) {
	rawForCanonical := map[string]string{
		"k8s.container.cpu.request":    "k8s.container.cpu_request",
		"k8s.container.cpu.limit":      "k8s.container.cpu_limit",
		"k8s.container.memory.request": "k8s.container.memory_request",
		"k8s.container.memory.limit":   "k8s.container.memory_limit",
	}
	for _, metric := range metrics.DefaultCatalog() {
		hasKubeCluster := false
		for _, source := range metric.Sources {
			if source.ID == metrics.SourceKubeCluster {
				hasKubeCluster = true
				break
			}
		}
		if !hasKubeCluster || metric.Name == "k8s.node.condition.ready" || metric.Name == "k8s.node.condition.memory_pressure" || metric.Name == "k8s.node.condition.disk_pressure" {
			continue
		}
		raw := metric.Name
		if mapped, ok := rawForCanonical[metric.Name]; ok {
			raw = mapped
		}
		_, ok := Parse(raw, nil)
		require.Truef(t, ok, "catalog metric %q needs a supported source parser path", metric.Name)
	}
}
