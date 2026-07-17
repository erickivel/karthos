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
	require.NotContains(t, string(payload), "selector")
}

func TestClusterProfileIncludesOTLPCapabilitiesAndSignalClasses(t *testing.T) {
	profile := ClusterProfile{
		RecommendedReceivers: []string{"kubeletstats", "otlp"},
		Sources: []ObservabilitySource{
			{
				Name:    "application-otlp",
				Type:    "otlp",
				Present: true,
				OTLP: &OTLPSource{
					MetricsEnabled: true,
					TracesEnabled:  true,
					GRPCEndpoint:   "0.0.0.0:4317",
					HTTPEndpoint:   "0.0.0.0:4318",
				},
			},
		},
		CollectionPaths: []OTelCollectionPath{
			{
				ReceiverType: "otlp",
				Enabled:      true,
				SignalTypes:  []string{"metric", "trace"},
			},
		},
	}

	payload, err := json.Marshal(profile)
	require.NoError(t, err)

	require.Contains(t, string(payload), "signal_types")
	require.Contains(t, string(payload), "grpc_endpoint")
	require.Contains(t, string(payload), "http_endpoint")
}
