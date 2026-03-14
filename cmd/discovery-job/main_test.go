package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/adapters/otel"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

func TestRunPlanWritesRenderedCollectorManifests(t *testing.T) {
	tempDir := t.TempDir()
	profilePath := filepath.Join(tempDir, "profile.json")
	outputPath := filepath.Join(tempDir, "otel-manifests.yaml")

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
	require.Contains(t, string(output), "kind: ConfigMap")
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
	err = run(context.Background(), []string{
		"--mode=deploy",
		"--profile=" + profilePath,
	}, dependencies{
		planner:   otel.NewSourcePlanner("oksg-system"),
		generator: defaultGenerator(),
		deployer: deployFunc(func(_ context.Context, manifests []byte) error {
			applied = string(manifests)
			return nil
		}),
	})
	require.NoError(t, err)
	require.Contains(t, applied, "kind: DaemonSet")
	require.Contains(t, applied, "kind: Deployment")
	require.NotContains(t, applied, "prometheus:")
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
