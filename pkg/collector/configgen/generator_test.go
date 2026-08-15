package configgen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/adapters/otel"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources/contract"
)

type testGenerator struct{ *Generator }

func newTestGenerator() testGenerator { return testGenerator{NewGenerator()} }
func newTestGeneratorWithOptions(options GeneratorOptions) testGenerator {
	return testGenerator{NewGeneratorWithOptions(options)}
}
func (g testGenerator) Generate(deployment otel.Deployment) ([]byte, error) {
	return g.Generator.Generate(deployment, profile.ClusterProfile{}, contract.ResolvedSourcePlan{})
}
func (g testGenerator) Build(deployment otel.Deployment) (*Values, error) {
	return g.Generator.Build(deployment, profile.ClusterProfile{}, contract.ResolvedSourcePlan{})
}

func TestGenerateBuildsHelmValuesFromCollectorPlan(t *testing.T) {
	renderer := newTestGenerator()

	values, err := renderer.Generate(otel.Deployment{
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
				"kubelet_stats": map[string]any{"collection_interval": "30s"},
				"prometheus":    map[string]any{"config": map[string]any{"scrape_configs": []any{}}},
			},
			Processors: map[string]any{"batch": map[string]any{}},
			Exporters:  map[string]any{"otlp": map[string]any{"endpoint": "otel-collector-gateway:4317"}},
			Service: otel.ServiceConfig{
				Extensions: []string{"health_check"},
				Pipelines: map[string]otel.PipelineConfig{
					"metrics": {
						Receivers:  []string{"kubelet_stats", "prometheus"},
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
	require.NotContains(t, string(values), "enableCPUUsageMetrics")
	require.Contains(t, string(values), "enabledSignals:")
	require.Contains(t, string(values), "- trace")
}

func TestBuildUsesV0156FeatureGates(t *testing.T) {
	renderer := newTestGenerator()
	values, err := renderer.Build(otel.Deployment{})
	require.NoError(t, err)

	require.Equal(t, "0.156.0", values.Global.Image.Tag)
	require.True(t, values.Agent.HostNetwork)
	require.Empty(t, values.Agent.FeatureGates)
	require.Empty(t, values.Gateway.FeatureGates)
}

func TestBuildEmbedsResolvedSourcePlanForHelmOwnedContract(t *testing.T) {
	resolved := contract.ResolvedSourcePlan{
		ProfileFingerprint: "profile-fingerprint",
		SelectedSources:    []string{"kubelet"},
		MetricAssignments:  map[string]string{"k8s.pod.cpu.usage": "kubelet"},
	}
	values, err := NewGenerator().Build(otel.Deployment{}, profile.ClusterProfile{}, resolved)
	require.NoError(t, err)
	require.Equal(t, resolved, values.Backend.SourceContract.SourcePlan)
}

func TestBuildEmbedsCapabilityProfileForHelmOwnedContract(t *testing.T) {
	capabilityProfile := profile.ClusterProfile{
		ClusterName: "kind-test",
		Capabilities: []profile.SourceCapability{{
			SourceID: "kubelet",
			Kind:     profile.SourceKindDirect,
			Status:   profile.CapabilityAvailable,
			Signals:  []string{"metric"},
		}},
	}
	values, err := NewGenerator().Build(otel.Deployment{}, capabilityProfile, contract.ResolvedSourcePlan{})
	require.NoError(t, err)
	require.Equal(t, capabilityProfile, values.Backend.SourceContract.CapabilityProfile)
}

func TestGenerateIncludesHostmetricsInAgentConfig(t *testing.T) {
	renderer := newTestGenerator()

	values, err := renderer.Generate(otel.Deployment{
		ClusterName:    "kind-test",
		Namespace:      "oksg-system",
		EnabledSignals: []string{"metric"},
		Backend: otel.BackendConfig{
			Enabled:      true,
			GRPCEndpoint: "oksg-backend:4317",
			HTTPEndpoint: "0.0.0.0:4318",
		},
		AgentConfig: otel.CollectorConfig{
			Receivers: map[string]any{
				"kubelet_stats": map[string]any{"collection_interval": "30s"},
				"host_metrics": map[string]any{
					"collection_interval": "30s",
					"root_path":           "/hostfs",
					"scrapers": map[string]any{
						"cpu":    map[string]any{},
						"memory": map[string]any{},
					},
				},
			},
			Processors: map[string]any{"batch": map[string]any{}},
			Exporters:  map[string]any{"otlp": map[string]any{"endpoint": "otel-collector-gateway:4317"}},
			Service: otel.ServiceConfig{
				Pipelines: map[string]otel.PipelineConfig{
					"metrics": {
						Receivers:  []string{"kubelet_stats", "host_metrics"},
						Processors: []string{"batch"},
						Exporters:  []string{"otlp"},
					},
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
	require.Contains(t, string(values), "host_metrics:")
	require.Contains(t, string(values), "root_path: /hostfs")
	require.Contains(t, string(values), "kubelet_stats:")
}

func TestGenerateIncludesK8sclusterInGatewayConfig(t *testing.T) {
	renderer := newTestGenerator()

	values, err := renderer.Generate(otel.Deployment{
		ClusterName:    "kind-test",
		Namespace:      "oksg-system",
		EnabledSignals: []string{"metric"},
		Backend: otel.BackendConfig{
			Enabled:      true,
			GRPCEndpoint: "oksg-backend:4317",
			HTTPEndpoint: "0.0.0.0:4318",
		},
		AgentConfig: otel.CollectorConfig{
			Receivers:  map[string]any{"kubelet_stats": map[string]any{"collection_interval": "30s"}},
			Processors: map[string]any{"batch": map[string]any{}},
			Exporters:  map[string]any{"otlp": map[string]any{"endpoint": "otel-collector-gateway:4317"}},
			Service: otel.ServiceConfig{
				Pipelines: map[string]otel.PipelineConfig{
					"metrics": {Receivers: []string{"kubelet_stats"}, Processors: []string{"batch"}, Exporters: []string{"otlp"}},
				},
			},
		},
		GatewayConfig: otel.CollectorConfig{
			Receivers: map[string]any{
				"otlp": map[string]any{"protocols": map[string]any{"grpc": map[string]any{}}},
				"k8s_cluster": map[string]any{
					"collection_interval":       "30s",
					"node_conditions_to_report": []string{"Ready", "MemoryPressure"},
				},
			},
			Processors: map[string]any{"batch": map[string]any{}},
			Exporters:  map[string]any{"otlp": map[string]any{"endpoint": "oksg-backend:4317"}},
			Service: otel.ServiceConfig{
				Pipelines: map[string]otel.PipelineConfig{
					"metrics": {Receivers: []string{"otlp", "k8s_cluster"}, Processors: []string{"batch"}, Exporters: []string{"otlp"}},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Contains(t, string(values), "k8s_cluster:")
	require.Contains(t, string(values), "node_conditions_to_report:")
}

func TestGenerateIncludesServicegraphConnectorWhenPresent(t *testing.T) {
	renderer := newTestGenerator()

	values, err := renderer.Generate(otel.Deployment{
		ClusterName:    "kind-test",
		Namespace:      "oksg-system",
		EnabledSignals: []string{"metric", "trace"},
		Backend: otel.BackendConfig{
			Enabled:      true,
			GRPCEndpoint: "oksg-backend:4317",
			HTTPEndpoint: "0.0.0.0:4318",
		},
		AgentConfig: otel.CollectorConfig{
			Receivers: map[string]any{
				"otlp":          map[string]any{"protocols": map[string]any{"grpc": map[string]any{}}},
				"kubelet_stats": map[string]any{"collection_interval": "30s"},
			},
			Processors: map[string]any{"batch": map[string]any{}},
			Connectors: map[string]any{
				"service_graph": map[string]any{
					"latency_histogram_buckets": []string{"100ms", "500ms", "1s"},
				},
			},
			Exporters: map[string]any{"otlp": map[string]any{"endpoint": "otel-collector-gateway:4317"}},
			Service: otel.ServiceConfig{
				Pipelines: map[string]otel.PipelineConfig{
					"metrics": {
						Receivers:  []string{"kubelet_stats", "service_graph"},
						Processors: []string{"batch"},
						Exporters:  []string{"otlp"},
					},
					"traces": {
						Receivers:  []string{"otlp"},
						Processors: []string{"batch"},
						Exporters:  []string{"otlp", "service_graph"},
					},
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
					"traces":  {Receivers: []string{"otlp"}, Processors: []string{"batch"}, Exporters: []string{"otlp"}},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Contains(t, string(values), "connectors:")
	require.NotContains(t, string(values), "span_metrics:")
	require.Contains(t, string(values), "service_graph:")
}

func TestGenerateBaselinePlanHasNoConnectorsSection(t *testing.T) {
	renderer := newTestGenerator()

	values, err := renderer.Generate(otel.Deployment{
		ClusterName:    "kind-test",
		Namespace:      "oksg-system",
		EnabledSignals: []string{"metric"},
		Backend: otel.BackendConfig{
			Enabled:      true,
			GRPCEndpoint: "oksg-backend:4317",
			HTTPEndpoint: "0.0.0.0:4318",
		},
		AgentConfig: otel.CollectorConfig{
			Receivers:  map[string]any{"kubelet_stats": map[string]any{"collection_interval": "30s"}},
			Processors: map[string]any{"batch": map[string]any{}},
			Exporters:  map[string]any{"otlp": map[string]any{"endpoint": "otel-collector-gateway:4317"}},
			Service: otel.ServiceConfig{
				Pipelines: map[string]otel.PipelineConfig{
					"metrics": {Receivers: []string{"kubelet_stats"}, Processors: []string{"batch"}, Exporters: []string{"otlp"}},
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
	require.NotContains(t, string(values), "connectors:")
}

func TestGenerateUsesConfiguredBackendImageOverrides(t *testing.T) {
	renderer := newTestGeneratorWithOptions(GeneratorOptions{
		BackendImage: ImageValues{
			Repository: "localhost:5001/oksg-backend",
			Tag:        "phase3",
			PullPolicy: "IfNotPresent",
		},
	})

	values, err := renderer.Generate(otel.Deployment{
		ClusterName:    "kind-test",
		Namespace:      "oksg-system",
		EnabledSignals: []string{"metric"},
		Backend: otel.BackendConfig{
			Enabled:      true,
			GRPCEndpoint: "oksg-backend:4317",
			HTTPEndpoint: "0.0.0.0:4318",
		},
		AgentConfig: otel.CollectorConfig{
			Receivers:  map[string]any{"kubelet_stats": map[string]any{}},
			Processors: map[string]any{"batch": map[string]any{}},
			Exporters:  map[string]any{"otlp": map[string]any{"endpoint": "otel-collector-gateway:4317"}},
			Service: otel.ServiceConfig{
				Pipelines: map[string]otel.PipelineConfig{
					"metrics": {Receivers: []string{"kubelet_stats"}, Processors: []string{"batch"}, Exporters: []string{"otlp"}},
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

func TestGenerateUsesConfiguredReleaseMetadataOverrides(t *testing.T) {
	renderer := newTestGeneratorWithOptions(GeneratorOptions{
		ReleaseName: "custom-otel",
		ChartPath:   "./deploy/helm/custom-otel",
	})

	values, err := renderer.Generate(otel.Deployment{
		ClusterName:    "kind-test",
		Namespace:      "oksg-system",
		EnabledSignals: []string{"metric"},
		Backend: otel.BackendConfig{
			Enabled:      true,
			GRPCEndpoint: "oksg-backend:4317",
			HTTPEndpoint: "0.0.0.0:4318",
		},
		AgentConfig: otel.CollectorConfig{
			Receivers:  map[string]any{"kubelet_stats": map[string]any{}},
			Processors: map[string]any{"batch": map[string]any{}},
			Exporters:  map[string]any{"otlp": map[string]any{"endpoint": "otel-collector-gateway:4317"}},
			Service: otel.ServiceConfig{
				Pipelines: map[string]otel.PipelineConfig{
					"metrics": {Receivers: []string{"kubelet_stats"}, Processors: []string{"batch"}, Exporters: []string{"otlp"}},
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
	require.Contains(t, string(values), "releaseName: custom-otel")
	require.Contains(t, string(values), "chartPath: ./deploy/helm/custom-otel")
	require.NotContains(t, string(values), "releaseName: oksg-otel")
}
