package profile

import (
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestClusterProfileIncludesRecommendedReceiversAndPrometheusTargets(t *testing.T) {
	profile := ClusterProfile{
		RecommendedReceivers: []string{"kubeletstats", "prometheus"},
		Sources: []ObservabilitySource{
			{
				Name:    "application-prometheus",
				Type:    "prometheus",
				Present: true,
				PrometheusTargets: []PrometheusTarget{
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
	}

	payload, err := json.Marshal(profile)
	require.NoError(t, err)

	require.Contains(t, string(payload), "recommended_receivers")
	require.Contains(t, string(payload), "prometheus_targets")
}
