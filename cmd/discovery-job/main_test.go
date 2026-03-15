package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/adapters/otel"
	"github.com/erickivel/oksg/pkg/collector/deployer"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

func TestRunPlanWritesRenderedCollectorManifests(t *testing.T) {
	tempDir := t.TempDir()
	profilePath := filepath.Join(tempDir, "profile.json")
	outputPath := filepath.Join(tempDir, "otel-values.yaml")

	err := os.WriteFile(profilePath, []byte(`{
  "sources": [{
    "name": "application-prometheus",
    "type": "prometheus",
    "present": true,
    "prometheus_targets": [{
      "service_name": "gateway-go",
      "namespace": "oksg-demo",
      "port": 8080,
      "path": "/metrics",
      "reachable": true
    }]
  }]
}`), 0o644)
	require.NoError(t, err)

	err = run(context.Background(), []string{
		"--mode=plan",
		"--profile=" + profilePath,
		"--output=" + outputPath,
	}, dependencies{
		planner:   otel.NewSourcePlanner("oksg-system"),
		generator: defaultGenerator(),
	})
	require.NoError(t, err)

	output, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	require.Contains(t, string(output), "collectorConfig")
	require.Contains(t, string(output), "prometheus:")
}

func TestRunDeployUsesGeneratedManifests(t *testing.T) {
	tempDir := t.TempDir()
	profilePath := filepath.Join(tempDir, "profile.json")

	err := os.WriteFile(profilePath, []byte(`{
  "sources": [{
    "name": "application-prometheus",
    "type": "prometheus",
    "present": false
  }]
}`), 0o644)
	require.NoError(t, err)

	var applied string
	var appliedOptions deployer.Options
	err = run(context.Background(), []string{
		"--mode=deploy",
		"--profile=" + profilePath,
	}, dependencies{
		planner:   otel.NewSourcePlanner("oksg-system"),
		generator: defaultGenerator(),
		deployer: deployFunc(func(_ context.Context, values []byte, options deployer.Options) error {
			applied = string(values)
			appliedOptions = options
			return nil
		}),
	})
	require.NoError(t, err)
	require.Contains(t, applied, "collectorConfig")
	require.Equal(t, "oksg-system", appliedOptions.Namespace)
	require.NotContains(t, applied, "prometheus:")
}

func TestRunDeployUsesHelmNamespaceForBothPlannerAndDeployer(t *testing.T) {
	tempDir := t.TempDir()
	profilePath := filepath.Join(tempDir, "profile.json")

	err := os.WriteFile(profilePath, []byte(`{
  "sources": [{
    "name": "application-prometheus",
    "type": "prometheus",
    "present": false
  }]
}`), 0o644)
	require.NoError(t, err)

	var applied string
	var appliedOptions deployer.Options
	err = run(context.Background(), []string{
		"--mode=deploy",
		"--profile=" + profilePath,
		"--namespace=default-ns",
		"--helm-namespace=override-ns",
	}, dependencies{
		generator: defaultGenerator(),
		deployer: deployFunc(func(_ context.Context, values []byte, options deployer.Options) error {
			applied = string(values)
			appliedOptions = options
			return nil
		}),
	})
	require.NoError(t, err)
	require.Equal(t, "override-ns", appliedOptions.Namespace)
	require.Contains(t, applied, "namespace: override-ns")
	require.NotContains(t, applied, "namespace: default-ns")
}

func TestRunDiscoverWritesProfile(t *testing.T) {
	tempDir := t.TempDir()
	outputPath := filepath.Join(tempDir, "profile.json")

	err := run(context.Background(), []string{
		"--mode=discover",
		"--output=" + outputPath,
	}, dependencies{
		discover: func(context.Context) (*profile.ClusterProfile, error) {
			return &profile.ClusterProfile{
				RecommendedReceivers: []string{"kubeletstats"},
			}, nil
		},
	})
	require.NoError(t, err)

	output, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	require.Contains(t, string(output), "recommended_receivers")
}
