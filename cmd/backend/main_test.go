package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/adapters/otel"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

func TestParseConfigAppliesDefaults(t *testing.T) {
	cfg, err := parseConfig([]string{})
	require.NoError(t, err)
	require.Equal(t, defaultBackendGRPCListenAddr, cfg.GRPCListenAddress)
	require.Equal(t, defaultBackendHTTPListenAddr, cfg.HTTPListenAddress)
	require.Equal(t, defaultBackendHealthListenAddr, cfg.HealthListenAddress)
	require.Equal(t, defaultBackendLogLevel, cfg.LogLevel)
	require.Equal(t, []string{"metric", "trace"}, cfg.EnabledSignals)
	require.Equal(t, defaultStorageBackend, cfg.StorageBackend)
	require.Equal(t, "", cfg.PostgresDSN)
	require.Equal(t, "", cfg.VMURL)
	require.False(t, cfg.RawStoreEnabled)
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
		"--raw-store-enabled",
	})
	require.NoError(t, err)
	require.Equal(t, "external", cfg.StorageBackend)
	require.Equal(t, "postgres://localhost/oksg", cfg.PostgresDSN)
	require.Equal(t, "http://localhost:8428", cfg.VMURL)
	require.True(t, cfg.RawStoreEnabled)
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
	dir := t.TempDir()
	profilePath := filepath.Join(dir, "capability-profile.json")
	planPath := filepath.Join(dir, "source-plan.json")
	capabilityProfile := profile.ClusterProfile{
		ClusterID:   entities.NewCluster("test", nil).ID(),
		ClusterName: "test",
		Capabilities: []profile.SourceCapability{{
			SourceID: metrics.SourceKubelet,
			Kind:     profile.SourceKindDirect,
			Status:   profile.CapabilityAvailable,
			Signals:  []string{"metrics"},
		}},
	}
	writeJSONFile(t, profilePath, capabilityProfile)
	writeJSONFile(t, planPath, otel.ResolvedSourcePlan{
		ProfileFingerprint: "wrong",
		SelectedSources:    []string{metrics.SourceKubelet},
		MetricAssignments:  metrics.Assignment{"system.cpu.utilization": metrics.SourceKubelet},
	})

	_, _, err := loadSourceContract(config{CapabilityProfile: profilePath, SourcePlan: planPath})
	require.ErrorContains(t, err, "fingerprint does not match")
}

func writeJSONFile(t *testing.T, path string, value any) {
	t.Helper()
	payload, err := json.Marshal(value)
	require.NoError(t, err)
	require.NoError(t, os.WriteFile(path, payload, 0o600))
}
