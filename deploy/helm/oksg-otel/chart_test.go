package oksgotel_test

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/adapters/otel"
	"github.com/erickivel/oksg/pkg/collector"
	"github.com/erickivel/oksg/pkg/collector/configgen"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources"
)

func TestRenderedChartUsesHelmReleaseIdentityAndRequiredCollectionPlane(t *testing.T) {
	rendered := renderChartWithRuntime(t, "project-observe", "observability", "chart-test", configgen.RuntimeSettings{
		Storage: configgen.StorageSettings{
			Backend:         configgen.StorageBackendExternal,
			PostgreSQL:      configgen.StoreSettings{Mode: configgen.StoreModeBundled},
			VictoriaMetrics: configgen.StoreSettings{Mode: configgen.StoreModeBundled},
		},
	})

	for _, resource := range []string{
		"name: project-observe-agent",
		"name: project-observe-gateway",
		"name: project-observe-backend",
		"name: project-observe-frontend",
		"name: project-observe-source-contract",
		"namespace: observability",
		"checksum/config:",
		"checksum/source-contract:",
		"hostNetwork: true",
		"OKSG_GATEWAY_GRPC_ENDPOINT",
		"project-observe-gateway:4317",
		"OKSG_BACKEND_GRPC_ENDPOINT",
		"project-observe-backend:4317",
		"project-observe-victoria-metrics-single-server:8428",
	} {
		require.Contains(t, rendered, resource)
	}
}

func TestRenderedChartUsesTypedFrontendImageOverride(t *testing.T) {
	rendered := renderChartWithRuntime(t, "oksg-otel", "oksg-system", "frontend-test", configgen.RuntimeSettings{
		FrontendImage: configgen.ImageValues{Repository: "registry.example/oksg-frontend", Tag: "e2e", PullPolicy: "Always"},
	})
	require.Contains(t, rendered, "image: registry.example/oksg-frontend:e2e")
	require.Contains(t, rendered, "imagePullPolicy: Always")
}

func TestRenderedChartPreservesAdapterOwnedCollectorDebugConfiguration(t *testing.T) {
	rendered := renderChartWithCompilerOptions(t, "oksg-otel", "oksg-system", "debug-test", configgen.RuntimeSettings{}, otel.CompilerOptions{DebugEnabled: true})
	for _, expected := range []string{
		"debug/oksg:",
		"verbosity: detailed",
		"use_internal_logger: false",
		"- stdout",
	} {
		require.Contains(t, rendered, expected)
	}
	require.Equal(t, 2, strings.Count(rendered, "debug/oksg:"), "both generated Collector ConfigMaps must retain the adapter-owned exporter")
}

func TestRenderedChartRejectsUnknownComponentSetting(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is required for chart-render integration coverage")
	}

	valuesPath := generatedValuesPath(t, "schema-test")
	command := exec.CommandContext(t.Context(), helm, "template", "oksg-otel", chartPath(t), "--namespace", "oksg-system", "-f", valuesPath, "--set", "gateway.unknownSetting=true")
	output, err := command.CombinedOutput()
	require.Errorf(t, err, "chart accepted an unsupported component setting: %s", output)
	require.Contains(t, string(output), "additional properties 'unknownSetting' not allowed")
}

func TestRenderedChartRejectsUnsupportedRootAndFrontendSettings(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is required for chart-render integration coverage")
	}

	valuesPath := generatedValuesPath(t, "schema-test")
	for _, testCase := range []struct {
		name  string
		value string
	}{
		{name: "unsupported root setting", value: "unsupportedRoot=true"},
		{name: "unsupported frontend setting", value: "frontend.unexpectedSetting=true"},
	} {
		t.Run(testCase.name, func(t *testing.T) {
			command := exec.CommandContext(t.Context(), helm, "template", "oksg-otel", chartPath(t), "--namespace", "oksg-system", "-f", valuesPath, "--set", testCase.value)
			output, err := command.CombinedOutput()
			require.Errorf(t, err, "chart accepted unsupported setting %q: %s", testCase.value, output)
			require.Contains(t, string(output), "additional properties")
		})
	}
}

func TestSourceContractChangeRollsBackend(t *testing.T) {
	initial := renderChart(t, "oksg-otel", "oksg-system", "contract-test")
	updated := renderChart(t, "oksg-otel", "oksg-system", "contract-test-updated")
	require.NotEqual(t, checksum(t, initial, "checksum/source-contract:"), checksum(t, updated, "checksum/source-contract:"))
}

func renderChart(t *testing.T, releaseName, namespace, clusterName string, extraArgs ...string) string {
	return renderChartWithRuntime(t, releaseName, namespace, clusterName, configgen.RuntimeSettings{}, extraArgs...)
}

func renderChartWithRuntime(t *testing.T, releaseName, namespace, clusterName string, runtime configgen.RuntimeSettings, extraArgs ...string) string {
	return renderChartWithCompilerOptions(t, releaseName, namespace, clusterName, runtime, otel.CompilerOptions{}, extraArgs...)
}

func renderChartWithCompilerOptions(t *testing.T, releaseName, namespace, clusterName string, runtime configgen.RuntimeSettings, compilerOptions otel.CompilerOptions, extraArgs ...string) string {
	t.Helper()
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is required for chart-render integration coverage")
	}
	valuesPath := generatedValuesPathWithRuntimeAndCompilerOptions(t, clusterName, runtime, compilerOptions)
	args := []string{"template", releaseName, chartPath(t), "--namespace", namespace, "-f", valuesPath}
	args = append(args, extraArgs...)
	command := exec.CommandContext(t.Context(), helm, args...)
	rendered, err := command.CombinedOutput()
	require.NoErrorf(t, err, "helm template failed: %s", rendered)
	return string(rendered)
}

func generatedValuesPath(t *testing.T, clusterName string) string {
	return generatedValuesPathWithRuntime(t, clusterName, configgen.RuntimeSettings{})
}

func generatedValuesPathWithRuntime(t *testing.T, clusterName string, runtime configgen.RuntimeSettings) string {
	return generatedValuesPathWithRuntimeAndCompilerOptions(t, clusterName, runtime, otel.CompilerOptions{})
}

func generatedValuesPathWithRuntimeAndCompilerOptions(t *testing.T, clusterName string, runtime configgen.RuntimeSettings, compilerOptions otel.CompilerOptions) string {
	t.Helper()
	capabilityProfile := profile.ClusterProfile{
		ClusterName: clusterName,
		Capabilities: []profile.SourceCapability{
			{SourceID: "kubernetes.structure", Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"structure"}},
			{SourceID: "kubelet", Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
			{SourceID: "host-os", Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
			{SourceID: "kube-cluster", Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
			{SourceID: "prometheus.app", Kind: profile.SourceKindDirect, Status: profile.CapabilityUnavailable},
			{SourceID: "mesh.istio", Kind: profile.SourceKindDirect, Status: profile.CapabilityUnavailable},
			{SourceID: "traces.servicegraph", Kind: profile.SourceKindDerived, Status: profile.CapabilityUnavailable},
		},
	}
	registry := sources.DefaultRegistry()
	sourcePlan, err := registry.ResolveSourcePlan(&capabilityProfile)
	require.NoError(t, err)
	collectionPlan, err := collector.NewComposer("oksg-system").Compose(&capabilityProfile, sourcePlan)
	require.NoError(t, err)
	deployment, err := otel.NewCompilerWithOptions(compilerOptions).Compile(collectionPlan)
	require.NoError(t, err)
	values, err := configgen.NewGeneratorWithOptions(configgen.GeneratorOptions{Runtime: runtime}).Generate(deployment, capabilityProfile, sourcePlan)
	require.NoError(t, err)

	valuesPath := filepath.Join(t.TempDir(), "values.yaml")
	require.NoError(t, os.WriteFile(valuesPath, values, 0o600))
	return valuesPath
}

func chartPath(t *testing.T) string {
	t.Helper()
	return "."
}

func checksum(t *testing.T, rendered, key string) string {
	t.Helper()
	for _, line := range strings.Split(rendered, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, key) {
			return strings.TrimSpace(strings.TrimPrefix(line, key))
		}
	}
	t.Fatalf("rendered chart has no %s annotation", key)
	return ""
}
