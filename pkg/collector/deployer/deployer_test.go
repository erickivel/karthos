package deployer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
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

func contains(values []string, needle string) bool {
	return strings.Contains(strings.Join(values, " "), needle)
}
