package logging

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestLoggerWritesStructuredJSONLine(t *testing.T) {
	var out bytes.Buffer

	logger, err := NewWithWriter("discovery-job", "info", &out)
	require.NoError(t, err)

	logger.Info("planning complete", String("mode", "plan"), String("cluster", "cluster:kind-test"))

	var entry map[string]any
	require.NoError(t, json.Unmarshal(out.Bytes(), &entry))
	require.Equal(t, "info", entry["level"])
	require.Equal(t, "discovery-job", entry["component"])
	require.Equal(t, "planning complete", entry["msg"])
	require.Equal(t, "plan", entry["mode"])
	require.Equal(t, "cluster:kind-test", entry["cluster"])
	require.NotEmpty(t, entry["ts"])
}

func TestLoggerRespectsLevelFiltering(t *testing.T) {
	var out bytes.Buffer

	logger, err := NewWithWriter("discovery-job", "warn", &out)
	require.NoError(t, err)

	logger.Info("ignored")
	logger.Error("visible")

	require.NotContains(t, out.String(), "ignored")
	require.Contains(t, out.String(), "visible")
}
