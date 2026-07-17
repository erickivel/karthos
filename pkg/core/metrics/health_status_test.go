package metrics

import (
	"math"
	"testing"

	"github.com/stretchr/testify/require"
)

func TestPodHealthLabel(t *testing.T) {
	cases := []struct {
		name     string
		value    float64
		expected string
	}{
		{"healthy", 1, HealthHealthy},
		{"running_not_ready", 2, HealthRunningNotReady},
		{"restarting", 3, HealthRestarting},
		{"pending", 4, HealthPending},
		{"succeeded", 5, HealthSucceeded},
		{"failed", 6, HealthFailed},
		{"unknown_explicit", 7, HealthUnknown},
		{"out_of_range_low", 0, HealthUnknown},
		{"out_of_range_high", 99, HealthUnknown},
		{"negative", -1, HealthUnknown},
		{"nan", math.NaN(), HealthUnknown},
		{"float_drift_above", 1.4, HealthHealthy},
		{"float_drift_below", 0.51, HealthHealthy},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.expected, PodHealthLabel(c.value))
		})
	}
}

func TestContainerHealthLabel(t *testing.T) {
	cases := []struct {
		name     string
		value    float64
		expected string
	}{
		{"healthy", 1, HealthHealthy},
		{"running_not_ready", 2, HealthRunningNotReady},
		{"restarting", 3, HealthRestarting},
		{"out_of_range_high", 4, HealthUnknown},
		{"out_of_range_low", 0, HealthUnknown},
		{"negative", -1, HealthUnknown},
		{"nan", math.NaN(), HealthUnknown},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			require.Equal(t, c.expected, ContainerHealthLabel(c.value))
		})
	}
}

func TestIsHealthStatusMetric(t *testing.T) {
	require.True(t, IsHealthStatusMetric("oksg.pod.health.status"))
	require.True(t, IsHealthStatusMetric("oksg.container.health.status"))
	require.True(t, IsHealthStatusMetric("k8s.pod.status_reason"))
	require.False(t, IsHealthStatusMetric("k8s.pod.phase"))
	require.False(t, IsHealthStatusMetric("k8s.container.ready"))
	require.False(t, IsHealthStatusMetric(""))
}

func TestHealthLabelDispatch(t *testing.T) {
	require.Equal(t, HealthHealthy, HealthLabel("oksg.pod.health.status", 1))
	require.Equal(t, HealthRestarting, HealthLabel("oksg.container.health.status", 3))
	require.Equal(t, "", HealthLabel("k8s.pod.phase", 2))
	require.Equal(t, "", HealthLabel("", 1))
}

func TestPodStatusReasonLabel(t *testing.T) {
	cases := map[float64]string{
		1: "evicted",
		2: "node_affinity",
		3: "node_lost",
		4: "shutdown",
		5: "unexpected_admission_error",
		6: "unknown",
	}
	for value, expected := range cases {
		require.Equal(t, expected, HealthLabel("k8s.pod.status_reason", value))
	}
}
