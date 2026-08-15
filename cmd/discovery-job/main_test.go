package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/adapters/otel"
	"github.com/erickivel/oksg/pkg/collector/configgen"
	"github.com/erickivel/oksg/pkg/collector/deployer"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources/contract"
)

func TestRunPlanWritesResolvedSourcePlanBesideValues(t *testing.T) {
	tempDir := t.TempDir()
	profilePath := filepath.Join(tempDir, "profile.json")
	valuesPath := filepath.Join(tempDir, "otel-values.yaml")
	require.NoError(t, os.WriteFile(profilePath, []byte(capabilityProfileJSON), 0o644))

	err := run(context.Background(), []string{
		"--mode=plan",
		"--capability-profile=" + profilePath,
		"--values-output=" + valuesPath,
	}, dependencies{planner: newCommandPlanner("oksg-system"), generator: defaultGenerator(configgen.GeneratorOptions{})})
	require.NoError(t, err)

	values, err := os.ReadFile(valuesPath)
	require.NoError(t, err)
	require.Contains(t, string(values), "collectorConfig")
	require.Contains(t, string(values), "sourceContract")
	require.Contains(t, string(values), "capabilityProfile")
	require.Contains(t, string(values), "sourcePlan")
	resolved, err := os.ReadFile(valuesPath + ".source-plan.json")
	require.NoError(t, err)
	require.Contains(t, string(resolved), "profile_fingerprint")
	require.Contains(t, string(resolved), "metric_assignments")
}

func TestRunDiscoverWritesCapabilityProfile(t *testing.T) {
	outputPath := filepath.Join(t.TempDir(), "profile.json")
	err := run(context.Background(), []string{"--mode=discover", "--output=" + outputPath}, dependencies{
		discover: func(context.Context) (*profile.ClusterProfile, error) {
			return &profile.ClusterProfile{Capabilities: []profile.SourceCapability{{
				SourceID: "kubelet", Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable,
			}}}, nil
		},
	})
	require.NoError(t, err)
	payload, err := os.ReadFile(outputPath)
	require.NoError(t, err)
	require.Contains(t, string(payload), "capabilities")
}

func TestRunDeployAppliesGeneratedHelmValuesAndWritesRequestedArtifact(t *testing.T) {
	tempDir := t.TempDir()
	profilePath := filepath.Join(tempDir, "profile.json")
	valuesPath := filepath.Join(tempDir, "otel-values.yaml")
	require.NoError(t, os.WriteFile(profilePath, []byte(capabilityProfileJSON), 0o644))

	generatedValues := []byte("backend:\n  sourceContract:\n    capabilityProfile: {}\n    sourcePlan: {}\n")
	deployer := &recordingDeployer{}
	err := run(context.Background(), []string{
		"--mode=deploy",
		"--capability-profile=" + profilePath,
		"--values-output=" + valuesPath,
	}, dependencies{
		planner:   staticPlanner{plan: &deploymentPlan{}},
		generator: staticGenerator{values: generatedValues},
		deployer:  deployer,
	})
	require.NoError(t, err)
	require.Equal(t, generatedValues, deployer.appliedValues)
	writtenValues, err := os.ReadFile(valuesPath)
	require.NoError(t, err)
	require.Equal(t, append(generatedValues, '\n'), writtenValues)
}

type staticPlanner struct{ plan *deploymentPlan }

func (p staticPlanner) Plan(context.Context, *profile.ClusterProfile) (*deploymentPlan, error) {
	return p.plan, nil
}

type staticGenerator struct{ values []byte }

func (g staticGenerator) Generate(otel.Deployment, profile.ClusterProfile, contract.ResolvedSourcePlan) ([]byte, error) {
	return g.values, nil
}

type recordingDeployer struct{ appliedValues []byte }

func (d *recordingDeployer) Apply(_ context.Context, values []byte, _ deployer.Options) error {
	d.appliedValues = append([]byte(nil), values...)
	return nil
}

func (d *recordingDeployer) Template(context.Context, []byte, deployer.Options) error { return nil }

const capabilityProfileJSON = `{
  "cluster_name": "kind-test",
  "capabilities": [
    {"source_id":"kubernetes.structure","kind":"direct","status":"available","signals":["structure"]},
    {"source_id":"kubelet","kind":"direct","status":"available","signals":["metric"]},
    {"source_id":"host-os","kind":"direct","status":"available","signals":["metric"]},
    {"source_id":"kube-cluster","kind":"direct","status":"available","signals":["metric"]},
    {"source_id":"prometheus.app","kind":"direct","status":"available","signals":["metric"],"evidence":{"prometheus_targets":[{"service_name":"gateway","namespace":"demo","port":8080,"path":"/metrics"}]}},
    {"source_id":"mesh.istio","kind":"direct","status":"unavailable"},
    {"source_id":"traces.servicegraph","kind":"derived","status":"unavailable"}
  ]
}`
