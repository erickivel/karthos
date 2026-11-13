package main

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sigs.k8s.io/yaml"
	"sort"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/karthos/pkg/adapters/otel"
	"github.com/erickivel/karthos/pkg/collector/configgen"
	"github.com/erickivel/karthos/pkg/collector/deployer"
	"github.com/erickivel/karthos/pkg/discovery/profile"
	"github.com/erickivel/karthos/pkg/sources/contract"
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
	}, dependencies{planner: newCommandPlanner("karthos-system", false), generator: defaultGenerator(configgen.GeneratorOptions{})})
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

func TestRunPlanCollectorDebugCompilesFixedDebugExporter(t *testing.T) {
	tempDir := t.TempDir()
	profilePath := filepath.Join(tempDir, "profile.json")
	valuesPath := filepath.Join(tempDir, "otel-values.yaml")
	require.NoError(t, os.WriteFile(profilePath, []byte(capabilityProfileJSON), 0o644))

	err := run(context.Background(), []string{
		"--mode=plan",
		"--capability-profile=" + profilePath,
		"--values-output=" + valuesPath,
		"--collector-debug",
	}, dependencies{})
	require.NoError(t, err)

	values, err := os.ReadFile(valuesPath)
	require.NoError(t, err)
	require.Contains(t, string(values), "debug/karthos:")
	require.Contains(t, string(values), "verbosity: detailed")
	require.Contains(t, string(values), "use_internal_logger: false")
	require.Contains(t, string(values), "- stdout")
}

func TestRunPlanCollectorDebugEnvironmentCompilesFixedDebugExporter(t *testing.T) {
	t.Setenv("KARTHOS_COLLECTOR_DEBUG", "true")
	tempDir := t.TempDir()
	profilePath := filepath.Join(tempDir, "profile.json")
	valuesPath := filepath.Join(tempDir, "otel-values.yaml")
	require.NoError(t, os.WriteFile(profilePath, []byte(capabilityProfileJSON), 0o644))

	err := run(context.Background(), []string{
		"--mode=plan",
		"--capability-profile=" + profilePath,
		"--values-output=" + valuesPath,
	}, dependencies{})
	require.NoError(t, err)

	values, err := os.ReadFile(valuesPath)
	require.NoError(t, err)
	require.Contains(t, string(values), "debug/karthos:")
}

func TestRunRejectsInvalidCollectorDebugEnvironment(t *testing.T) {
	t.Setenv("KARTHOS_COLLECTOR_DEBUG", "not-a-boolean")
	err := run(context.Background(), []string{"--mode=discover"}, dependencies{})
	require.ErrorContains(t, err, "KARTHOS_COLLECTOR_DEBUG must be a boolean")
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

func TestRunDeployCompilesTypedExternalStorageAndFrontendImageSettings(t *testing.T) {
	tempDir := t.TempDir()
	profilePath := filepath.Join(tempDir, "profile.json")
	valuesPath := filepath.Join(tempDir, "otel-values.yaml")
	require.NoError(t, os.WriteFile(profilePath, []byte(capabilityProfileJSON), 0o644))

	deployer := &recordingDeployer{}
	err := run(context.Background(), []string{
		"--mode=deploy",
		"--capability-profile=" + profilePath,
		"--values-output=" + valuesPath,
		"--storage-backend=external",
		"--postgresql-mode=bundled",
		"--victoriametrics-mode=bundled",
		"--frontend-image-repository=registry.example/karthos-frontend",
		"--frontend-image-tag=e2e",
		"--frontend-image-pull-policy=Always",
	}, dependencies{deployer: deployer})
	require.NoError(t, err)
	require.Contains(t, string(deployer.appliedValues), "backend: external")
	require.Contains(t, string(deployer.appliedValues), "postgresql:")
	require.Contains(t, string(deployer.appliedValues), "enabled: true")
	require.Contains(t, string(deployer.appliedValues), "repository: registry.example/karthos-frontend")
	require.Contains(t, string(deployer.appliedValues), "tag: e2e")
}

func TestRunDeployRejectsIncompleteExternalStorageBeforeHelm(t *testing.T) {
	tempDir := t.TempDir()
	profilePath := filepath.Join(tempDir, "profile.json")
	require.NoError(t, os.WriteFile(profilePath, []byte(capabilityProfileJSON), 0o644))

	deployer := &recordingDeployer{}
	err := run(context.Background(), []string{
		"--mode=deploy",
		"--capability-profile=" + profilePath,
		"--storage-backend=external",
		"--postgresql-mode=bundled",
	}, dependencies{deployer: deployer})
	require.ErrorContains(t, err, "victoriametrics mode is required for external storage")
	require.Empty(t, deployer.appliedValues)
}

func TestRunDeployCompilesSupportedStorageModes(t *testing.T) {
	testCases := []struct {
		name       string
		arguments  []string
		expectText []string
	}{
		{
			name:       "memory",
			arguments:  []string{"--storage-backend=memory"},
			expectText: []string{"backend: memory", "enabled: false"},
		},
		{
			name:       "bundled PostgreSQL and VictoriaMetrics",
			arguments:  []string{"--storage-backend=external", "--postgresql-mode=bundled", "--victoriametrics-mode=bundled"},
			expectText: []string{"backend: external", "postgresql:", "victoriametrics:", "enabled: true"},
		},
		{
			name:       "remote PostgreSQL and bundled VictoriaMetrics",
			arguments:  []string{"--storage-backend=external", "--postgresql-mode=remote", "--postgresql-dsn=postgres://remote", "--victoriametrics-mode=bundled"},
			expectText: []string{"dsn: postgres://remote", "victoriametrics:", "enabled: true"},
		},
		{
			name:       "bundled PostgreSQL and remote VictoriaMetrics",
			arguments:  []string{"--storage-backend=external", "--postgresql-mode=bundled", "--victoriametrics-mode=remote", "--victoriametrics-url=https://vm.example"},
			expectText: []string{"postgresql:", "enabled: true", "url: https://vm.example"},
		},
	}

	for _, testCase := range testCases {
		t.Run(testCase.name, func(t *testing.T) {
			tempDir := t.TempDir()
			profilePath := filepath.Join(tempDir, "profile.json")
			require.NoError(t, os.WriteFile(profilePath, []byte(capabilityProfileJSON), 0o644))
			deployer := &recordingDeployer{}
			arguments := append([]string{"--mode=deploy", "--capability-profile=" + profilePath}, testCase.arguments...)

			require.NoError(t, run(context.Background(), arguments, dependencies{deployer: deployer}))
			for _, expected := range testCase.expectText {
				require.Contains(t, string(deployer.appliedValues), expected)
			}
		})
	}
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

func TestRunPlanServicegraphStoreCapacityAndBuckets(t *testing.T) {
	for _, tc := range []struct {
		name string
		flag string
		want int
	}{
		{"default", "", 10000}, {"negative-control", "1000", 1000}, {"supported", "10000", 10000}, {"override", "12000", 12000},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var capabilityProfile profile.ClusterProfile
			require.NoError(t, json.Unmarshal([]byte(capabilityProfileJSON), &capabilityProfile))
			capabilityProfile.Capabilities[6] = profile.SourceCapability{SourceID: "traces.servicegraph", Kind: profile.SourceKindDerived, Status: profile.CapabilityAvailable, Signals: []string{"metric"}, Evidence: profile.CapabilityEvidence{OTLP: &profile.OTLPEvidence{TracesEnabled: true}}}
			dir := t.TempDir()
			profilePath := filepath.Join(dir, "profile.json")
			valuesPath := filepath.Join(dir, "values.yaml")
			payload, err := json.Marshal(capabilityProfile)
			require.NoError(t, err)
			require.NoError(t, os.WriteFile(profilePath, payload, 0o600))
			args := []string{"--mode=plan", "--capability-profile=" + profilePath, "--values-output=" + valuesPath}
			if tc.flag != "" {
				args = append(args, "--servicegraph-store-max-items="+tc.flag)
			}
			require.NoError(t, run(context.Background(), args, dependencies{}))
			payload, err = os.ReadFile(valuesPath)
			require.NoError(t, err)
			var values configgen.Values
			require.NoError(t, yaml.Unmarshal(payload, &values))
			var gateway struct {
				Connectors map[string]struct {
					Buckets []string `json:"latency_histogram_buckets"`
					Flush   string   `json:"metrics_flush_interval"`
					Store   struct {
						MaxItems int    `json:"max_items"`
						TTL      string `json:"ttl"`
					} `json:"store"`
				} `json:"connectors"`
			}
			require.NoError(t, yaml.Unmarshal([]byte(values.Gateway.CollectorYAML), &gateway))
			connector := gateway.Connectors["service_graph"]
			require.Equal(t, tc.want, connector.Store.MaxItems)
			require.Equal(t, "30s", connector.Store.TTL)
			require.Equal(t, "15s", connector.Flush)
			require.Equal(t, []string{"100ms", "250ms", "500ms", "1s", "5s"}, connector.Buckets)
			// v0.156 uses SearchFloat64s directly on emitted bounds. An 800ms sample
			// must occupy the 1s bucket. Live collector tests verify sum/count.
			bounds := make([]float64, len(connector.Buckets))
			for i, b := range connector.Buckets {
				d, err := time.ParseDuration(b)
				require.NoError(t, err)
				bounds[i] = d.Seconds()
			}
			require.Equal(t, 3, sort.SearchFloat64s(bounds, 0.8))
		})
	}
}

func TestRunRejectsInvalidServicegraphStoreCapacity(t *testing.T) {
	for _, value := range []string{"0", "-1", "not-an-int", "1.5", "999999999999999999999999999"} {
		t.Run(value, func(t *testing.T) {
			err := run(context.Background(), []string{"--mode=plan", fmt.Sprintf("--servicegraph-store-max-items=%s", value)}, dependencies{})
			require.Error(t, err)
			require.Contains(t, err.Error(), "servicegraph-store-max-items")
			if value == "0" || value == "-1" {
				require.Contains(t, err.Error(), "positive")
			}
		})
	}
}
