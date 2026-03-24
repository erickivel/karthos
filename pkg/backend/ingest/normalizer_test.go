package ingest_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/backend/ingest"
	"github.com/erickivel/oksg/pkg/core/schema"
)

const testCluster = "test-cluster"

func marshalPayload(rm *metricspb.ResourceMetrics) json.RawMessage {
	b, _ := protojson.Marshal(rm)
	return json.RawMessage(b)
}

func makeEnvelope(t *testing.T, resource map[string]string, rm *metricspb.ResourceMetrics) schema.RawEnvelope {
	t.Helper()
	env, err := schema.NewRawEnvelope(
		"otelcol",
		schema.SignalMetric,
		resource,
		marshalPayload(rm),
		time.Now().UTC(),
	)
	require.NoError(t, err)
	return env
}

func podResource() map[string]string {
	return map[string]string{
		"k8s.pod.name":       "test-pod",
		"k8s.namespace.name": "default",
	}
}

func makeGaugeRM(metricName string, value float64, tsNano uint64) *metricspb.ResourceMetrics {
	return &metricspb.ResourceMetrics{
		ScopeMetrics: []*metricspb.ScopeMetrics{
			{
				Metrics: []*metricspb.Metric{
					{
						Name: metricName,
						Data: &metricspb.Metric_Gauge{
							Gauge: &metricspb.Gauge{
								DataPoints: []*metricspb.NumberDataPoint{
									{
										Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: value},
										TimeUnixNano: tsNano,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func makeGaugeRMWithAttr(metricName string, value float64, tsNano uint64, attrKey, attrVal string) *metricspb.ResourceMetrics {
	return &metricspb.ResourceMetrics{
		ScopeMetrics: []*metricspb.ScopeMetrics{
			{
				Metrics: []*metricspb.Metric{
					{
						Name: metricName,
						Data: &metricspb.Metric_Gauge{
							Gauge: &metricspb.Gauge{
								DataPoints: []*metricspb.NumberDataPoint{
									{
										Attributes: []*commonpb.KeyValue{
											{
												Key: attrKey,
												Value: &commonpb.AnyValue{
													Value: &commonpb.AnyValue_StringValue{StringValue: attrVal},
												},
											},
										},
										Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: value},
										TimeUnixNano: tsNano,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func makeSumRMInt(metricName string, value int64, tsNano uint64) *metricspb.ResourceMetrics {
	return &metricspb.ResourceMetrics{
		ScopeMetrics: []*metricspb.ScopeMetrics{
			{
				Metrics: []*metricspb.Metric{
					{
						Name: metricName,
						Data: &metricspb.Metric_Sum{
							Sum: &metricspb.Sum{
								DataPoints: []*metricspb.NumberDataPoint{
									{
										Value:        &metricspb.NumberDataPoint_AsInt{AsInt: value},
										TimeUnixNano: tsNano,
									},
								},
							},
						},
					},
				},
			},
		},
	}
}

func newNormalizer() *ingest.OTLPNormalizer {
	resolver := ingest.NewOTLPEntityResolver(testCluster)
	return ingest.NewOTLPNormalizer(resolver)
}

func TestOTLPNormalizer_KubeletstatsCP(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC).UnixNano())

	rm := makeGaugeRM("k8s.pod.cpu.usage", 0.5, tsNano)
	env := makeEnvelope(t, podResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	s := samples[0]
	assert.Equal(t, "cpu_usage_millicores", s.MetricName)
	assert.InDelta(t, 500.0, s.Value, 1e-9)
	assert.Equal(t, "otelcol", s.Labels["source"])
	assert.Equal(t, time.Unix(0, int64(tsNano)).UTC(), s.Timestamp)
}

func TestOTLPNormalizer_MemoryMetric(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRM("k8s.pod.memory.working_set", 1048576, tsNano)
	env := makeEnvelope(t, podResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	s := samples[0]
	assert.Equal(t, "memory_usage_bytes", s.MetricName)
	assert.InDelta(t, 1048576.0, s.Value, 1e-9)
}

func TestOTLPNormalizer_DiskDirection(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	t.Run("read direction", func(t *testing.T) {
		rm := makeGaugeRMWithAttr("system.disk.io", 1000, tsNano, "direction", "read")
		env := makeEnvelope(t, podResource(), rm)

		samples, err := normalizer.Normalize(context.Background(), env)
		require.NoError(t, err)
		require.Len(t, samples, 1)
		assert.Equal(t, "disk_read_bps", samples[0].MetricName)
	})

	t.Run("write direction", func(t *testing.T) {
		rm := makeGaugeRMWithAttr("system.disk.io", 2000, tsNano, "direction", "write")
		env := makeEnvelope(t, podResource(), rm)

		samples, err := normalizer.Normalize(context.Background(), env)
		require.NoError(t, err)
		require.Len(t, samples, 1)
		assert.Equal(t, "disk_write_bps", samples[0].MetricName)
	})
}

func TestOTLPNormalizer_NetworkDirection(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	t.Run("transmit direction", func(t *testing.T) {
		rm := makeGaugeRMWithAttr("system.network.io", 5000, tsNano, "direction", "transmit")
		env := makeEnvelope(t, podResource(), rm)

		samples, err := normalizer.Normalize(context.Background(), env)
		require.NoError(t, err)
		require.Len(t, samples, 1)
		assert.Equal(t, "net_tx_bps", samples[0].MetricName)
	})

	t.Run("receive direction", func(t *testing.T) {
		rm := makeGaugeRMWithAttr("system.network.io", 3000, tsNano, "direction", "receive")
		env := makeEnvelope(t, podResource(), rm)

		samples, err := normalizer.Normalize(context.Background(), env)
		require.NoError(t, err)
		require.Len(t, samples, 1)
		assert.Equal(t, "net_rx_bps", samples[0].MetricName)
	})

	t.Run("tx direction", func(t *testing.T) {
		rm := makeGaugeRMWithAttr("system.network.io", 4000, tsNano, "direction", "tx")
		env := makeEnvelope(t, podResource(), rm)

		samples, err := normalizer.Normalize(context.Background(), env)
		require.NoError(t, err)
		require.Len(t, samples, 1)
		assert.Equal(t, "net_tx_bps", samples[0].MetricName)
	})
}

func TestOTLPNormalizer_UnmappedMetricSkipped(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRM("some.unknown.metric", 42.0, tsNano)
	env := makeEnvelope(t, podResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, samples)
}

func TestOTLPNormalizer_EntityResolutionError(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRM("k8s.pod.cpu.usage", 0.5, tsNano)
	// Resource with no recognized attributes
	env := makeEnvelope(t, map[string]string{"some.unknown.attr": "value"}, rm)

	_, err := normalizer.Normalize(context.Background(), env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve entity")
}

func TestOTLPNormalizer_SumDataPointInt64(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	// Use k8s.replicaset.desired which maps to replica_count with factor 1.0
	rm := makeSumRMInt("k8s.replicaset.desired", 3, tsNano)
	env := makeEnvelope(t, podResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	s := samples[0]
	assert.Equal(t, "replica_count", s.MetricName)
	assert.InDelta(t, 3.0, s.Value, 1e-9)
}
