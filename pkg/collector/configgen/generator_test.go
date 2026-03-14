package configgen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/adapters/otel"
)

func TestGenerateBuildsHelmValuesFromCollectorPlan(t *testing.T) {
	renderer := NewGenerator()

	values, err := renderer.Generate(otel.Plan{
		ClusterName: "kind-test",
		Namespace:   "oksg-system",
		AgentConfig: otel.CollectorConfig{
			Extensions: map[string]any{
				"health_check": map[string]any{"endpoint": "0.0.0.0:13133"},
			},
			Receivers: map[string]any{
				"kubeletstats": map[string]any{"collection_interval": "30s"},
				"prometheus":   map[string]any{"config": map[string]any{"scrape_configs": []any{}}},
			},
			Processors: map[string]any{"batch": map[string]any{}},
			Exporters:  map[string]any{"otlp": map[string]any{"endpoint": "otel-collector-gateway:4317"}},
			Service: otel.ServiceConfig{
				Extensions: []string{"health_check"},
				Pipelines: map[string]otel.PipelineConfig{
					"metrics": {
						Receivers:  []string{"kubeletstats", "prometheus"},
						Processors: []string{"batch"},
						Exporters:  []string{"otlp"},
					},
				},
			},
		},
		GatewayConfig: otel.CollectorConfig{
			Extensions: map[string]any{
				"health_check": map[string]any{"endpoint": "0.0.0.0:13133"},
			},
			Receivers:  map[string]any{"otlp": map[string]any{"protocols": map[string]any{"grpc": map[string]any{}}}},
			Processors: map[string]any{"batch": map[string]any{}},
			Exporters:  map[string]any{"debug": map[string]any{}},
			Service: otel.ServiceConfig{
				Extensions: []string{"health_check"},
				Pipelines: map[string]otel.PipelineConfig{
					"metrics": {
						Receivers:  []string{"otlp"},
						Processors: []string{"batch"},
						Exporters:  []string{"debug"},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Contains(t, string(values), "clusterName: kind-test")
	require.Contains(t, string(values), "collectorConfig: |")
	require.Contains(t, string(values), "health_check:")
	require.Contains(t, string(values), "prometheus:")
	require.Contains(t, string(values), "format: json")
	require.Contains(t, string(values), "releaseName: oksg-otel")
}
