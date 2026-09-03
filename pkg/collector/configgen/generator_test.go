package configgen

import (
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/adapters/otel"
	collectorspec "github.com/erickivel/oksg/pkg/collector/spec"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources/contract"
)

func TestGenerateEmitsDynamicValuesAndExplicitRuntimeOverrides(t *testing.T) {
	renderer := NewGeneratorWithOptions(GeneratorOptions{Runtime: RuntimeSettings{
		CollectorImage: ImageValues{Tag: "0.156.1"},
		BackendImage:   ImageValues{Repository: "registry.example/oksg-backend", Tag: "test", PullPolicy: "Always"},
		FrontendImage:  ImageValues{Repository: "registry.example/oksg-frontend", Tag: "test", PullPolicy: "Always"},
		Storage: StorageSettings{
			Backend:         StorageBackendExternal,
			PostgreSQL:      StoreSettings{Mode: StoreModeBundled},
			VictoriaMetrics: StoreSettings{Mode: StoreModeRemote, Endpoint: "https://vm.example"},
		},
	}})

	values, err := renderer.Generate(testDeployment(t), profile.ClusterProfile{ClusterName: "kind-test"}, contract.ResolvedSourcePlan{ProfileFingerprint: "fingerprint"})
	require.NoError(t, err)

	generated := string(values)
	require.Contains(t, generated, "clusterName: kind-test")
	require.Contains(t, generated, "collectorConfig: |")
	require.Contains(t, generated, "      telemetry:\n        logs:\n          encoding: json\n          level: info\n          output_paths:\n          - stdout")
	require.Contains(t, generated, "sourceContract:")
	require.Contains(t, generated, "tag: 0.156.1")
	require.Contains(t, generated, "repository: registry.example/oksg-frontend")
	require.Contains(t, generated, "backend: external")
	require.Contains(t, generated, "url: https://vm.example")
}

func TestGenerateRejectsIncompleteExternalStorageSettings(t *testing.T) {
	renderer := NewGeneratorWithOptions(GeneratorOptions{Runtime: RuntimeSettings{Storage: StorageSettings{
		Backend:    StorageBackendExternal,
		PostgreSQL: StoreSettings{Mode: StoreModeBundled},
	}}})

	_, err := renderer.Generate(testDeployment(t), profile.ClusterProfile{}, contract.ResolvedSourcePlan{})
	require.ErrorContains(t, err, "victoriametrics mode is required for external storage")
}

func TestBuildEmbedsSourceContract(t *testing.T) {
	capabilityProfile := profile.ClusterProfile{ClusterName: "kind-test", Capabilities: []profile.SourceCapability{{
		SourceID: "kubelet", Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"},
	}}}
	resolved := contract.ResolvedSourcePlan{
		ProfileFingerprint: "profile-fingerprint",
		SelectedSources:    []string{"kubelet"},
		MetricAssignments:  map[string]string{"k8s.pod.cpu.usage": "kubelet"},
	}

	values, err := NewGenerator().Build(testDeployment(t), capabilityProfile, resolved)
	require.NoError(t, err)
	require.Equal(t, capabilityProfile, values.Backend.SourceContract.CapabilityProfile)
	require.Equal(t, resolved, values.Backend.SourceContract.SourcePlan)
}

func testDeployment(t *testing.T) otel.Deployment {
	t.Helper()

	deployment, err := otel.NewCompiler().Compile(collectorspec.CollectionPlan{
		ClusterName: "kind-test",
		Namespace:   "oksg-system",
		Agent:       collectorspec.AgentRequirements{OTLP: collectorspec.OTLPReceiver{Metrics: true}},
		Gateway:     collectorspec.GatewayRequirements{OTLP: collectorspec.OTLPReceiver{Metrics: true}},
	})
	require.NoError(t, err)
	return deployment
}
