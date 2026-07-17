package ingest_test

import (
	"context"

	"testing"
	"time"

	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOTLPNormalizer_HostSemanticLabels(t *testing.T) {
	tests := []struct {
		name       string
		metricName string
		attrs      map[string]string
		wantLabels map[string]string
		absent     []string
	}{
		{
			name:       "cpu",
			metricName: "system.cpu.time",
			attrs:      map[string]string{"cpu": "cpu3", "state": "system"},
			wantLabels: map[string]string{"cpu": "cpu3", "state": "system"},
		},
		{
			name:       "network",
			metricName: "system.network.io",
			attrs:      map[string]string{"device": "enp7s0", "direction": "receive"},
			wantLabels: map[string]string{"network_interface_name": "enp7s0", "direction": "receive"},
			absent:     []string{"device"},
		},
		{
			name:       "disk",
			metricName: "system.disk.operations",
			attrs:      map[string]string{"device": "dm-0", "direction": "write"},
			wantLabels: map[string]string{"device": "dm-0", "direction": "write"},
		},
		{
			name:       "filesystem",
			metricName: "system.filesystem.usage",
			attrs: map[string]string{
				"device": "/dev/dm-0", "mountpoint": "/", "type": "ext4", "mode": "rw", "state": "used",
			},
			wantLabels: map[string]string{
				"device": "/dev/dm-0", "mountpoint": "/", "filesystem_type": "ext4", "filesystem_mode": "rw", "state": "used",
			},
			absent: []string{"type", "mode"},
		},
		{
			name:       "paging",
			metricName: "system.paging.operations",
			attrs: map[string]string{
				"device": "swap0", "type": "cached", "direction": "page_out", "state": "used",
			},
			wantLabels: map[string]string{
				"device": "swap0", "paging_type": "cached", "direction": "page_out", "state": "used",
			},
			absent: []string{"type"},
		},
		{
			name:       "connections",
			metricName: "system.network.connections",
			attrs:      map[string]string{"protocol": "tcp", "state": "established"},
			wantLabels: map[string]string{"protocol": "tcp", "state": "established"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalizer := newNormalizer()
			rm := makeSumRMIntWithAttrs(tt.metricName, 4096, uint64(time.Now().UnixNano()), tt.attrs,
				metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, true)
			samples, err := normalizer.Normalize(context.Background(), makeEnvelope(t, podResource(), rm))
			require.NoError(t, err)
			require.Len(t, samples, 1)
			for key, want := range tt.wantLabels {
				assert.Equal(t, want, samples[0].Labels[key], key)
			}
			for _, key := range tt.absent {
				assert.NotContains(t, samples[0].Labels, key)
			}
		})
	}
}

func TestOTLPNormalizer_PagingCountsUndoV0156GopsutilScale(t *testing.T) {
	for _, metricName := range []string{"system.paging.faults", "system.paging.operations"} {
		t.Run(metricName, func(t *testing.T) {
			normalizer := newNormalizer()
			rm := makeSumRMIntWithAttrs(metricName, 10*4096, uint64(time.Now().UnixNano()), map[string]string{"type": "major"},
				metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, true)
			samples, err := normalizer.Normalize(context.Background(), makeEnvelope(t, podResource(), rm))
			require.NoError(t, err)
			require.Len(t, samples, 1)
			assert.Equal(t, 10.0, samples[0].Value)
		})
	}
}

func TestOTLPNormalizer_PagingUsageIsNotScaled(t *testing.T) {
	normalizer := newNormalizer()
	rm := makeGaugeRMWithAttrs("system.paging.usage", 4096, uint64(time.Now().UnixNano()), map[string]string{"state": "used"})
	samples, err := normalizer.Normalize(context.Background(), makeEnvelope(t, podResource(), rm))
	require.NoError(t, err)
	require.Len(t, samples, 1)
	assert.Equal(t, 4096.0, samples[0].Value)
}

func TestOTLPNormalizer_DiskDirection(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	t.Run("read direction", func(t *testing.T) {
		rm := makeSumRMIntWithAttrs("system.disk.io", 1000, tsNano, map[string]string{"device": "dm-0", "direction": "read"},
			metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, true)
		env := makeEnvelope(t, podResource(), rm)

		samples, err := normalizer.Normalize(context.Background(), env)
		require.NoError(t, err)
		require.Len(t, samples, 1)
		assert.Equal(t, "system.disk.io.read", samples[0].MetricName)
	})

	t.Run("write direction", func(t *testing.T) {
		rm := makeSumRMIntWithAttrs("system.disk.io", 2000, tsNano, map[string]string{"device": "dm-0", "direction": "write"},
			metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, true)
		env := makeEnvelope(t, podResource(), rm)

		samples, err := normalizer.Normalize(context.Background(), env)
		require.NoError(t, err)
		require.Len(t, samples, 1)
		assert.Equal(t, "system.disk.io.write", samples[0].MetricName)
	})
}

func TestOTLPNormalizer_NetworkDirection(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	t.Run("transmit direction", func(t *testing.T) {
		rm := makeSumRMIntWithAttrs("system.network.io", 5000, tsNano, map[string]string{"device": "eth0", "direction": "transmit"},
			metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, true)
		env := makeEnvelope(t, podResource(), rm)

		samples, err := normalizer.Normalize(context.Background(), env)
		require.NoError(t, err)
		require.Len(t, samples, 1)
		assert.Equal(t, "system.network.io.transmit", samples[0].MetricName)
	})

	t.Run("receive direction", func(t *testing.T) {
		rm := makeSumRMIntWithAttrs("system.network.io", 3000, tsNano, map[string]string{"device": "eth0", "direction": "receive"},
			metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, true)
		env := makeEnvelope(t, podResource(), rm)

		samples, err := normalizer.Normalize(context.Background(), env)
		require.NoError(t, err)
		require.Len(t, samples, 1)
		assert.Equal(t, "system.network.io.receive", samples[0].MetricName)
	})

	t.Run("tx direction", func(t *testing.T) {
		rm := makeSumRMIntWithAttrs("system.network.io", 4000, tsNano, map[string]string{"device": "eth0", "direction": "tx"},
			metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, true)
		env := makeEnvelope(t, podResource(), rm)

		samples, err := normalizer.Normalize(context.Background(), env)
		require.NoError(t, err)
		require.Len(t, samples, 1)
		assert.Equal(t, "system.network.io.transmit", samples[0].MetricName)
	})
}
