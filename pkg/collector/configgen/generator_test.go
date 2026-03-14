package configgen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/adapters/otel"
)

func TestGenerateIncludesCollectorCoreResources(t *testing.T) {
	renderer := NewGenerator()

	manifests, err := renderer.Generate(otel.Plan{
		Namespace: "oksg-system",
		AgentConfig: otel.CollectorConfig{
			Receivers: map[string]any{
				"kubeletstats": map[string]any{"collection_interval": "30s"},
				"prometheus":   map[string]any{"config": map[string]any{"scrape_configs": []any{}}},
			},
			Processors: map[string]any{"batch": map[string]any{}},
			Exporters:  map[string]any{"otlp": map[string]any{"endpoint": "otel-collector-gateway:4317"}},
			Service: otel.ServiceConfig{
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
			Receivers:  map[string]any{"otlp": map[string]any{"protocols": map[string]any{"grpc": map[string]any{}}}},
			Processors: map[string]any{"batch": map[string]any{}},
			Exporters:  map[string]any{"debug": map[string]any{}},
			Service: otel.ServiceConfig{
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
	require.Contains(t, string(manifests), "kind: ConfigMap")
	require.Contains(t, string(manifests), "kind: DaemonSet")
	require.Contains(t, string(manifests), "kind: Deployment")
	require.Contains(t, string(manifests), "prometheus:")
	require.Contains(t, string(manifests), "kubeletstats:")
}
