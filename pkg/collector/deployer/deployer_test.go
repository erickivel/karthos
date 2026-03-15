package deployer

import (
	"context"
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

func contains(values []string, needle string) bool {
	return strings.Contains(strings.Join(values, " "), needle)
}
