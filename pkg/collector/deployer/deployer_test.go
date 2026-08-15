package deployer

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/adapters/otel"
	"github.com/erickivel/oksg/pkg/collector"
	"github.com/erickivel/oksg/pkg/collector/configgen"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources"
)

func TestDeployUsesHelmLintTemplateAndUpgrade(t *testing.T) {
	var commands [][]string

	deployer := New(func(_ context.Context, name string, args []string) error {
		commands = append(commands, append([]string{name}, args...))
		return nil
	})

	err := deployer.Apply(context.Background(), []byte("global:\n  namespace: oksg-system\n"), Options{
		ReleaseName:  "oksg-otel",
		ChartPath:    "./deploy/helm/oksg-otel",
		Namespace:    "oksg-system",
		Timeout:      30 * time.Second,
		DryRun:       true,
		RenderedPath: "bin/e2e/rendered",
	})
	require.NoError(t, err)
	require.Len(t, commands, 3)
	require.Equal(t, "helm", commands[0][0])
	require.Equal(t, []string{"helm", "lint", "./deploy/helm/oksg-otel"}, commands[0][:3])
	require.Equal(t, []string{"helm", "template", "oksg-otel", "./deploy/helm/oksg-otel"}, commands[1][:4])
	require.Equal(t, []string{"helm", "upgrade", "--install", "oksg-otel", "./deploy/helm/oksg-otel"}, commands[2][:5])
	require.True(t, contains(commands[2], "--dry-run"))
}

func TestTemplateRequiresOutputDirectory(t *testing.T) {
	deployer := New(func(_ context.Context, name string, args []string) error {
		return nil
	})

	err := deployer.Template(context.Background(), []byte("global:\n  namespace: oksg-system\n"), Options{})
	require.Error(t, err)
	require.Contains(t, err.Error(), "rendered output path is required")
}

func TestChartEnablesVictoriaMetricsRetryDeduplication(t *testing.T) {
	valuesPath := filepath.Join("..", "..", "..", "deploy", "helm", "oksg-otel", "values.yaml")
	values, err := os.ReadFile(valuesPath)
	require.NoError(t, err)
	require.Contains(t, string(values), "dedup.minScrapeInterval: 1ms")
}

func TestChartHostNetworkAccuracyContract(t *testing.T) {
	chartRoot := filepath.Join("..", "..", "..", "deploy", "helm", "oksg-otel")
	values, err := os.ReadFile(filepath.Join(chartRoot, "values.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(values), "hostNetwork: true")

	daemonSet, err := os.ReadFile(filepath.Join(chartRoot, "templates", "agent-daemonset.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(daemonSet), "hostNetwork: {{ .Values.agent.hostNetwork }}")
	require.Contains(t, string(daemonSet), `dnsPolicy: {{ ternary "ClusterFirstWithHostNet" "ClusterFirst" .Values.agent.hostNetwork }}`)

	configMap, err := os.ReadFile(filepath.Join(chartRoot, "templates", "agent-configmap.yaml"))
	require.NoError(t, err)
	require.NotContains(t, string(configMap), "hostNetwork")
	require.Contains(t, string(configMap), "generated collector-plan values")
}

func TestChartRequiresGeneratedCollectorConfiguration(t *testing.T) {
	valuesPath := filepath.Join("..", "..", "..", "deploy", "helm", "oksg-otel", "values.yaml")
	valuesRaw, err := os.ReadFile(valuesPath)
	require.NoError(t, err)
	require.NotContains(t, string(valuesRaw), "collectorConfig:")
	chartRoot := filepath.Join("..", "..", "..", "deploy", "helm", "oksg-otel", "templates")
	agent, err := os.ReadFile(filepath.Join(chartRoot, "agent-configmap.yaml"))
	require.NoError(t, err)
	gateway, err := os.ReadFile(filepath.Join(chartRoot, "gateway-configmap.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(agent), "required")
	require.Contains(t, string(gateway), "required")
}

func TestChartOwnsAndRollsTheCompiledSourceContract(t *testing.T) {
	chartRoot := filepath.Join("..", "..", "..", "deploy", "helm", "oksg-otel")
	contractTemplate, err := os.ReadFile(filepath.Join(chartRoot, "templates", "source-contract-configmap.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(contractTemplate), "capability-profile.json")
	require.Contains(t, string(contractTemplate), "source-plan.json")
	require.Contains(t, string(contractTemplate), ".Values.backend.sourceContract.capabilityProfile")
	require.Contains(t, string(contractTemplate), ".Values.backend.sourceContract.sourcePlan")

	backend, err := os.ReadFile(filepath.Join(chartRoot, "templates", "backend-deployment.yaml"))
	require.NoError(t, err)
	require.Contains(t, string(backend), "checksum/source-contract")
	require.Contains(t, string(backend), "--capability-profile=/etc/oksg/capability-profile.json")
	require.Contains(t, string(backend), "--source-plan=/etc/oksg/source-plan.json")

	valuesSchema, err := os.ReadFile(filepath.Join(chartRoot, "values.schema.json"))
	require.NoError(t, err)
	require.Contains(t, string(valuesSchema), "sourceContract")
	require.Contains(t, string(valuesSchema), "capabilityProfile")
	require.Contains(t, string(valuesSchema), "sourcePlan")
}

func TestRenderedChartValidatesAndRollsCompiledSourceContract(t *testing.T) {
	helm, err := exec.LookPath("helm")
	if err != nil {
		t.Skip("helm is required for chart-render integration coverage")
	}

	initial := renderedSourceContractChart(t, helm, "contract-test")
	require.Contains(t, initial, "kind: ConfigMap")
	require.Contains(t, initial, "name: oksg-otel-source-contract")
	require.Contains(t, initial, "capability-profile.json: |-")
	require.Contains(t, initial, "source-plan.json: |-")
	require.Contains(t, initial, "--capability-profile=/etc/oksg/capability-profile.json")
	require.Contains(t, initial, "--source-plan=/etc/oksg/source-plan.json")
	require.Contains(t, initial, "mountPath: /etc/oksg")

	updated := renderedSourceContractChart(t, helm, "contract-test-updated")
	require.NotEqual(t, renderedContractChecksum(t, initial), renderedContractChecksum(t, updated))
}

func renderedSourceContractChart(t *testing.T, helm, clusterName string) string {
	t.Helper()
	capabilityProfile := &profile.ClusterProfile{
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
	sourcePlan, err := registry.ResolveSourcePlan(capabilityProfile)
	require.NoError(t, err)
	collectionPlan, err := collector.NewComposer("oksg-system").Compose(capabilityProfile, sourcePlan)
	require.NoError(t, err)
	plan, err := otel.NewCompiler().Compile(collectionPlan)
	require.NoError(t, err)
	values, err := configgen.NewGenerator().Generate(plan, *capabilityProfile, sourcePlan)
	require.NoError(t, err)

	valuesPath := filepath.Join(t.TempDir(), "values.yaml")
	require.NoError(t, os.WriteFile(valuesPath, values, 0o600))
	chartPath := filepath.Join("..", "..", "..", "deploy", "helm", "oksg-otel")
	command := exec.CommandContext(t.Context(), helm, "template", "oksg-otel", chartPath, "--namespace", "oksg-system", "-f", valuesPath)
	rendered, err := command.CombinedOutput()
	require.NoErrorf(t, err, "helm template failed: %s", rendered)
	return string(rendered)
}

func renderedContractChecksum(t *testing.T, rendered string) string {
	t.Helper()
	for _, line := range strings.Split(rendered, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "checksum/source-contract:") {
			return strings.TrimSpace(strings.TrimPrefix(line, "checksum/source-contract:"))
		}
	}
	t.Fatal("rendered backend deployment has no source-contract checksum")
	return ""
}

func contains(values []string, needle string) bool {
	return strings.Contains(strings.Join(values, " "), needle)
}
