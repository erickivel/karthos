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
