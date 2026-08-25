package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources"
	"github.com/erickivel/oksg/pkg/sources/contract"
)

func TestParseConfigAppliesDefaults(t *testing.T) {
	cfg, err := parseConfig([]string{})
	require.NoError(t, err)
	require.Equal(t, defaultBackendGRPCListenAddr, cfg.GRPCListenAddress)
	require.Equal(t, defaultBackendHealthListenAddr, cfg.HealthListenAddress)
	require.Equal(t, defaultBackendLogLevel, cfg.LogLevel)
	require.Equal(t, []string{"metric", "trace"}, cfg.EnabledSignals)
	require.Equal(t, defaultStorageBackend, cfg.StorageBackend)
	require.Equal(t, "", cfg.PostgresDSN)
	require.Equal(t, "", cfg.VMURL)
}

func TestParseConfigAllowsSignalOverride(t *testing.T) {
	cfg, err := parseConfig([]string{"--enabled-signals=metric"})
	require.NoError(t, err)
	require.Equal(t, []string{"metric"}, cfg.EnabledSignals)
}

func TestParseConfigRejectsUnknownSignal(t *testing.T) {
	_, err := parseConfig([]string{"--enabled-signals=metric,log"})
	require.Error(t, err)
	require.ErrorContains(t, err, "unsupported signal")
}

func TestParseConfigStorageFlags(t *testing.T) {
	cfg, err := parseConfig([]string{
		"--storage-backend=external",
		"--postgres-dsn=postgres://localhost/oksg",
		"--victoriametrics-url=http://localhost:8428",
	})
	require.NoError(t, err)
	require.Equal(t, "external", cfg.StorageBackend)
	require.Equal(t, "postgres://localhost/oksg", cfg.PostgresDSN)
	require.Equal(t, "http://localhost:8428", cfg.VMURL)
}

func TestParseConfigParsesNodeTTLAndFlushInterval(t *testing.T) {
	cfg, err := parseConfig([]string{
		"--topology-flush-interval=30s",
		"--node-ttl=250ms",
		"--structure-refresh-interval=100ms",
	})
	require.NoError(t, err)
	require.Equal(t, 30*time.Second, cfg.TopologyFlushInterval)
	require.Equal(t, 250*time.Millisecond, cfg.NodeTTL)
}

func TestParseConfigParsesPodLifecycleRequired(t *testing.T) {
	cfg, err := parseConfig([]string{"--pod-lifecycle-required=true"})
	require.NoError(t, err)
	require.True(t, cfg.PodLifecycleRequired)
}

func TestValidateStructureRefreshIntervalRequiresStrictNodeTTLBound(t *testing.T) {
	require.NoError(t, validateStructureRefreshInterval(10*time.Minute, 24*time.Hour))
	require.ErrorContains(t, validateStructureRefreshInterval(24*time.Hour, 24*time.Hour), "strictly less")
	require.ErrorContains(t, validateStructureRefreshInterval(0, 24*time.Hour), "must be greater than zero")
}

func TestLoadSourceContractRejectsMismatchedProfileFingerprint(t *testing.T) {
	capabilityProfile, sourcePlan := completeSourceContract(t)
	sourcePlan.ProfileFingerprint = "wrong"
	profilePath, planPath := writeSourceContract(t, capabilityProfile, sourcePlan)

	_, _, err := loadSourceContract(config{CapabilityProfile: profilePath, SourcePlan: planPath})
	require.ErrorContains(t, err, "fingerprint does not match")
}

func TestLoadSourceContractAcceptsCompleteCompiledContract(t *testing.T) {
	capabilityProfile, sourcePlan := completeSourceContract(t)
	profilePath, planPath := writeSourceContract(t, capabilityProfile, sourcePlan)

	loadedProfile, loadedPlan, err := loadSourceContract(config{CapabilityProfile: profilePath, SourcePlan: planPath})
	require.NoError(t, err)
	require.Equal(t, capabilityProfile.ClusterID, loadedProfile.ClusterID)
	require.Equal(t, sourcePlan.MetricAssignments, loadedPlan.MetricAssignments)
}

func TestLoadSourceContractRejectsProfileWithoutEveryCompiledModule(t *testing.T) {
	capabilityProfile, sourcePlan := completeSourceContract(t)
	capabilityProfile.Capabilities = capabilityProfile.Capabilities[:len(capabilityProfile.Capabilities)-1]
	fingerprint, err := capabilityProfile.Fingerprint()
	require.NoError(t, err)
	sourcePlan.ProfileFingerprint = fingerprint
	profilePath, planPath := writeSourceContract(t, capabilityProfile, sourcePlan)

	_, _, err = loadSourceContract(config{CapabilityProfile: profilePath, SourcePlan: planPath})
	require.ErrorContains(t, err, "missing compiled module")
}

func TestLoadSourceContractRejectsIncompleteMetricAssignments(t *testing.T) {
	capabilityProfile, sourcePlan := completeSourceContract(t)
	for metricName := range sourcePlan.MetricAssignments {
		delete(sourcePlan.MetricAssignments, metricName)
		break
	}
	profilePath, planPath := writeSourceContract(t, capabilityProfile, sourcePlan)

	_, _, err := loadSourceContract(config{CapabilityProfile: profilePath, SourcePlan: planPath})
	require.ErrorContains(t, err, "must assign every available catalog metric")
}

func TestLoadSourceContractRejectsUnknownMetricAssignment(t *testing.T) {
	capabilityProfile, sourcePlan := completeSourceContract(t)
	sourcePlan.MetricAssignments["not.a.canonical.metric"] = metrics.SourceKubelet
	profilePath, planPath := writeSourceContract(t, capabilityProfile, sourcePlan)

	_, _, err := loadSourceContract(config{CapabilityProfile: profilePath, SourcePlan: planPath})
	require.ErrorContains(t, err, "assigns unknown catalog metric")
}

func TestLoadSourceContractRejectsUnavailableSelectedSource(t *testing.T) {
	capabilityProfile, sourcePlan := completeSourceContract(t)
	sourcePlan.SelectedSources = append(sourcePlan.SelectedSources, metrics.SourceMeshIstio)
	profilePath, planPath := writeSourceContract(t, capabilityProfile, sourcePlan)

	_, _, err := loadSourceContract(config{CapabilityProfile: profilePath, SourcePlan: planPath})
	require.ErrorContains(t, err, "selects unavailable source")
}

func TestLoadSourceContractRejectsAssignmentToUnselectedSource(t *testing.T) {
	capabilityProfile, sourcePlan := completeSourceContract(t)
	sourcePlan.SelectedSources = sourcePlan.SelectedSources[1:]
	profilePath, planPath := writeSourceContract(t, capabilityProfile, sourcePlan)

	_, _, err := loadSourceContract(config{CapabilityProfile: profilePath, SourcePlan: planPath})
	require.ErrorContains(t, err, "to unselected source")
}

func completeSourceContract(t *testing.T) (profile.ClusterProfile, contract.ResolvedSourcePlan) {
	t.Helper()
	capabilityProfile := profile.ClusterProfile{
		ClusterID:   entities.NewCluster("test", nil).ID(),
		ClusterName: "test",
		Capabilities: []profile.SourceCapability{
			{SourceID: metrics.SourceKubernetesStructure, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"structure"}},
			{SourceID: metrics.SourceKubelet, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
			{SourceID: metrics.SourceHostOS, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
			{SourceID: metrics.SourceKubeCluster, Kind: profile.SourceKindDirect, Status: profile.CapabilityAvailable, Signals: []string{"metric"}},
			{SourceID: metrics.SourcePrometheusApp, Kind: profile.SourceKindDirect, Status: profile.CapabilityUnavailable},
			{SourceID: metrics.SourceMeshIstio, Kind: profile.SourceKindDirect, Status: profile.CapabilityUnavailable},
			{SourceID: metrics.SourceTracesServicegraph, Kind: profile.SourceKindDerived, Status: profile.CapabilityUnavailable},
		},
	}
	planned, err := sources.DefaultRegistry().ResolveSourcePlan(&capabilityProfile)
	require.NoError(t, err)
	return capabilityProfile, planned
}

func writeSourceContract(t *testing.T, capabilityProfile profile.ClusterProfile, sourcePlan contract.ResolvedSourcePlan) (string, string) {
	t.Helper()
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "capability-profile.json")
	planPath := filepath.Join(dir, "source-plan.json")
	writeJSONFile(t, profilePath, capabilityProfile)
	writeJSONFile(t, planPath, sourcePlan)
	return profilePath, planPath
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, payload, 0o600))
}
