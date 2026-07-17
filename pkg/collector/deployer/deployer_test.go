package deployer

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"sigs.k8s.io/yaml"

	"github.com/erickivel/oksg/pkg/adapters/otel"
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

func TestChartUsesOTLPGRPCExporterName(t *testing.T) {
	valuesPath := filepath.Join("..", "..", "..", "deploy", "helm", "oksg-otel", "values.yaml")
	values, err := os.ReadFile(valuesPath)
	require.NoError(t, err)

	require.Equal(t, 2, strings.Count(string(values), "exporters:\n      otlp_grpc:"))
	require.NotContains(t, string(values), "exporters:\n      otlp:")
	require.NotContains(t, string(values), "exporters: [otlp]")
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
	require.Contains(t, string(configMap), "if .Values.agent.hostNetwork")
	require.Contains(t, string(configMap), `unset $scrapers "network"`)
}

func TestChartDefaultCollectorConfigsIsolateMetricProvenance(t *testing.T) {
	valuesPath := filepath.Join("..", "..", "..", "deploy", "helm", "oksg-otel", "values.yaml")
	valuesRaw, err := os.ReadFile(valuesPath)
	require.NoError(t, err)

	var values struct {
		Agent struct {
			CollectorConfig string `yaml:"collectorConfig"`
		} `yaml:"agent"`
		Gateway struct {
			CollectorConfig string `yaml:"collectorConfig"`
		} `yaml:"gateway"`
	}
	require.NoError(t, yaml.Unmarshal(valuesRaw, &values))

	var agent, gateway otel.CollectorConfig
	require.NoError(t, yaml.Unmarshal([]byte(values.Agent.CollectorConfig), &agent))
	require.NoError(t, yaml.Unmarshal([]byte(values.Gateway.CollectorConfig), &gateway))

	const sourceTagger = "transform/oksg-source"
	require.Contains(t, agent.Processors, sourceTagger)
	agentTagger, err := yaml.Marshal(agent.Processors[sourceTagger])
	require.NoError(t, err)
	require.Contains(t, string(agentTagger), `datapoint.attributes["oksg_source"]`)
	require.Contains(t, string(agentTagger), `"kubelet"`)
	require.Contains(t, string(agentTagger), `"host-os"`)
	require.Equal(t, []string{"kubelet_stats", "host_metrics"}, agent.Service.Pipelines["metrics"].Receivers)
	require.Contains(t, agent.Service.Pipelines["metrics"].Processors, sourceTagger)
	require.Equal(t, []string{"otlp"}, agent.Service.Pipelines["metrics/otlp"].Receivers)
	require.NotContains(t, agent.Service.Pipelines["metrics/otlp"].Processors, sourceTagger)

	require.Contains(t, gateway.Receivers, "k8s_cluster")
	require.Contains(t, gateway.Processors, sourceTagger)
	gatewayTagger, err := yaml.Marshal(gateway.Processors[sourceTagger])
	require.NoError(t, err)
	require.Contains(t, string(gatewayTagger), `datapoint.attributes["oksg_source"]`)
	require.Contains(t, string(gatewayTagger), `"kube-cluster"`)
	require.Equal(t, []string{"k8s_cluster"}, gateway.Service.Pipelines["metrics"].Receivers)
	require.Contains(t, gateway.Service.Pipelines["metrics"].Processors, sourceTagger)
	require.Equal(t, []string{"otlp"}, gateway.Service.Pipelines["metrics/otlp"].Receivers)
	require.NotContains(t, gateway.Service.Pipelines["metrics/otlp"].Processors, sourceTagger)
}

func contains(values []string, needle string) bool {
	return strings.Contains(strings.Join(values, " "), needle)
}
