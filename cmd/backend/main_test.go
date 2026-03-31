package main

import (
	"testing"

	"github.com/stretchr/testify/require"
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
