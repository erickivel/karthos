package configgen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/adapters/otel"
)

func TestGenerateBuildsHelmValuesFromCollectorPlan(t *testing.T) {
	renderer := NewGenerator()

	values, err := renderer.Generate(otel.Plan{
		ClusterName:    "kind-test",
		Namespace:      "oksg-system",
		EnabledSignals: []string{"metric", "trace"},
		Backend: otel.BackendConfig{
			Enabled:      true,
			GRPCEndpoint: "oksg-backend:4317",
			HTTPEndpoint: "0.0.0.0:4318",
		},
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
					"traces": {
						Receivers:  []string{"otlp"},
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
			Exporters:  map[string]any{"otlp": map[string]any{"endpoint": "oksg-backend:4317"}},
			Service: otel.ServiceConfig{
				Extensions: []string{"health_check"},
				Pipelines: map[string]otel.PipelineConfig{
					"metrics": {
						Receivers:  []string{"otlp"},
						Processors: []string{"batch"},
						Exporters:  []string{"otlp"},
					},
					"traces": {
						Receivers:  []string{"otlp"},
						Processors: []string{"batch"},
						Exporters:  []string{"otlp"},
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
	require.Contains(t, string(values), "backend:")
	require.Contains(t, string(values), "grpcEndpoint: oksg-backend:4317")
	require.Contains(t, string(values), "featureGates:")
	require.Contains(t, string(values), "receiver.kubeletstats.enableCPUUsageMetrics")
	require.Contains(t, string(values), "enabledSignals:")
	require.Contains(t, string(values), "- trace")
}

func TestGenerateUsesConfiguredBackendImageOverrides(t *testing.T) {
	renderer := NewGeneratorWithOptions(GeneratorOptions{
		BackendImage: ImageValues{
			Repository: "localhost:5001/oksg-backend",
			Tag:        "phase3",
			PullPolicy: "IfNotPresent",
		},
	})

	values, err := renderer.Generate(otel.Plan{
		ClusterName:    "kind-test",
		Namespace:      "oksg-system",
		EnabledSignals: []string{"metric"},
		Backend: otel.BackendConfig{
			Enabled:      true,
			GRPCEndpoint: "oksg-backend:4317",
			HTTPEndpoint: "0.0.0.0:4318",
		},
		AgentConfig: otel.CollectorConfig{
			Receivers:  map[string]any{"kubeletstats": map[string]any{}},
			Processors: map[string]any{"batch": map[string]any{}},
			Exporters:  map[string]any{"otlp": map[string]any{"endpoint": "otel-collector-gateway:4317"}},
			Service: otel.ServiceConfig{
				Pipelines: map[string]otel.PipelineConfig{
					"metrics": {Receivers: []string{"kubeletstats"}, Processors: []string{"batch"}, Exporters: []string{"otlp"}},
				},
			},
		},
		GatewayConfig: otel.CollectorConfig{
			Receivers:  map[string]any{"otlp": map[string]any{"protocols": map[string]any{"grpc": map[string]any{}}}},
			Processors: map[string]any{"batch": map[string]any{}},
			Exporters:  map[string]any{"otlp": map[string]any{"endpoint": "oksg-backend:4317"}},
			Service: otel.ServiceConfig{
				Pipelines: map[string]otel.PipelineConfig{
					"metrics": {Receivers: []string{"otlp"}, Processors: []string{"batch"}, Exporters: []string{"otlp"}},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Contains(t, string(values), "repository: localhost:5001/oksg-backend")
	require.Contains(t, string(values), "tag: phase3")
}
