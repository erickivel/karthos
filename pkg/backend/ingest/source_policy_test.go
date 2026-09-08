package ingest_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"

	"github.com/erickivel/oksg/pkg/backend/ingest"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/sources"
)

func makeNumberDataPoint(value float64, attrs map[string]string) *metricspb.NumberDataPoint {
	var kvs []*commonpb.KeyValue
	for key, val := range attrs {
		kvs = append(kvs, &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: val}}})
	}
	return &metricspb.NumberDataPoint{
		Attributes: kvs,
		Value:      &metricspb.NumberDataPoint_AsDouble{AsDouble: value},
	}
}

func TestOTLPNormalizerRequiresRegisteredMetricSource(t *testing.T) {
	tests := []struct {
		name  string
		attrs map[string]string
	}{
		{name: "missing", attrs: nil},
		{name: "unknown", attrs: map[string]string{"oksg.source": "not-registered"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			normalizer := newNormalizer()
			rm := makeGaugeRMWithAttrs("k8s.pod.cpu.usage", 1, uint64(time.Now().UnixNano()), tt.attrs)
			samples, err := normalizer.Normalize(context.Background(), makeEnvelopeWithoutSourceStamp(t, podResource(), rm))
			require.NoError(t, err)
			assert.Empty(t, samples)
		})
	}
}

func TestOTLPNormalizerSourceAssignmentAndMixedBatch(t *testing.T) {
	policy := ingest.NewMetricSourcePolicy(sources.DefaultRegistry().MetricIDs(), metrics.Assignment{
		"k8s.pod.cpu.usage": metrics.SourceKubelet,
	})
	normalizer := ingest.NewOTLPNormalizer(
		ingest.NewOTLPEntityResolver(testCluster),
		sources.DefaultRegistry(),
		ingest.WithSourcePolicy(policy),
	)

	rm := makeGaugeRMWithAttrs("k8s.pod.cpu.usage", 1, uint64(time.Now().UnixNano()), map[string]string{
		"oksg.source": metrics.SourceKubelet,
	})
	metric := rm.ScopeMetrics[0].Metrics[0]
	metric.GetGauge().DataPoints = append(metric.GetGauge().DataPoints,
		makeNumberDataPoint(2, map[string]string{"oksg_source": metrics.SourceMeshIstio}),
		makeNumberDataPoint(3, nil),
	)

	samples, err := normalizer.Normalize(context.Background(), makeEnvelopeWithoutSourceStamp(t, podResource(), rm))
	require.NoError(t, err)
	require.Len(t, samples, 1)
	assert.Equal(t, metrics.SourceKubelet, samples[0].Labels["source"])
}

func TestOTLPNormalizerUnknownMetricIsSkipped(t *testing.T) {
	normalizer := newNormalizer()
	rm := makeGaugeRM("vendor.unknown.metric", 1, uint64(time.Now().UnixNano()))
	samples, err := normalizer.Normalize(context.Background(), makeEnvelopeWithoutSourceStamp(t, podResource(), rm))
	require.NoError(t, err)
	assert.Empty(t, samples)
}

func TestOTLPNormalizerRejectsWrongSourceForSpecializedCanonicalMetrics(t *testing.T) {
	tests := []struct {
		name       string
		metricName string
		attrs      map[string]string
		assignment metrics.Assignment
	}{
		{
			name:       "directional disk counter",
			metricName: "system.disk.io",
			attrs: map[string]string{
				"direction":   "read",
				"oksg.source": metrics.SourceKubelet,
			},
			assignment: metrics.Assignment{
				"system.disk.io.read":  metrics.SourceHostOS,
				"system.disk.io.write": metrics.SourceHostOS,
			},
		},
		{
			name:       "node condition",
			metricName: "k8s.node.condition",
			attrs: map[string]string{
				"condition":   "Ready",
				"oksg.source": metrics.SourceKubelet,
			},
			assignment: metrics.Assignment{
				"k8s.node.condition.ready":           metrics.SourceKubeCluster,
				"k8s.node.condition.memory_pressure": metrics.SourceKubeCluster,
				"k8s.node.condition.disk_pressure":   metrics.SourceKubeCluster,
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			policy := ingest.NewMetricSourcePolicy(sources.DefaultRegistry().MetricIDs(), tt.assignment)
			normalizer := ingest.NewOTLPNormalizer(
				ingest.NewOTLPEntityResolver(testCluster),
				sources.DefaultRegistry(),
				ingest.WithSourcePolicy(policy),
			)
			rm := makeGaugeRMWithAttrs(tt.metricName, 1, uint64(time.Now().UnixNano()), tt.attrs)
			samples, err := normalizer.Normalize(context.Background(), makeEnvelopeWithoutSourceStamp(t, podResource(), rm))
			require.NoError(t, err)
			assert.Empty(t, samples)
		})
	}
}

func TestOTLPNormalizerAcceptsAssignedStampedNodeCondition(t *testing.T) {
	policy := ingest.NewMetricSourcePolicy(sources.DefaultRegistry().MetricIDs(), metrics.Assignment{
		"k8s.node.condition.ready":           metrics.SourceKubeCluster,
		"k8s.node.condition.memory_pressure": metrics.SourceKubeCluster,
		"k8s.node.condition.disk_pressure":   metrics.SourceKubeCluster,
	})
	normalizer := ingest.NewOTLPNormalizer(
		ingest.NewOTLPEntityResolver(testCluster),
		sources.DefaultRegistry(),
		ingest.WithSourcePolicy(policy),
	)
	rm := makeGaugeRMWithAttrs("k8s.node.condition", 1, uint64(time.Now().UnixNano()), map[string]string{"condition": "Ready"})
	stampMetricSource(rm, metrics.SourceKubeCluster)

	samples, err := normalizer.Normalize(context.Background(), makeEnvelopeWithoutSourceStamp(t, podResource(), rm))
	require.NoError(t, err)
	require.Len(t, samples, 1)
	require.Equal(t, "k8s.node.condition.ready", samples[0].MetricName)
	require.Equal(t, metrics.SourceKubeCluster, samples[0].Labels["source"])
}

func TestMetricSourcePolicyRejectsAmbiguousDescendantAssignments(t *testing.T) {
	policy := ingest.NewMetricSourcePolicy(sources.DefaultRegistry().MetricIDs(), metrics.Assignment{
		"system.disk.io.read":  metrics.SourceHostOS,
		"system.disk.io.write": metrics.SourceKubelet,
	})
	attrs := makeNumberDataPoint(1, map[string]string{"oksg.source": metrics.SourceHostOS}).Attributes

	validation := policy.Validate("system.disk.io", attrs)
	assert.False(t, validation.Accepted())
	assert.Equal(t, ingest.SourceRejectionAssignmentMismatch, validation.Reason)
}

func TestMetricSourcePolicyExactAssignmentPrecedesConflictingDescendants(t *testing.T) {
	policy := ingest.NewMetricSourcePolicy(sources.DefaultRegistry().MetricIDs(), metrics.Assignment{
		"system.disk.io":       metrics.SourceHostOS,
		"system.disk.io.read":  metrics.SourceHostOS,
		"system.disk.io.write": metrics.SourceKubelet,
	})
	hostAttrs := makeNumberDataPoint(1, map[string]string{"oksg.source": metrics.SourceHostOS}).Attributes
	kubeletAttrs := makeNumberDataPoint(1, map[string]string{"oksg.source": metrics.SourceKubelet}).Attributes

	assert.True(t, policy.Validate("system.disk.io", hostAttrs).Accepted())
	assert.Equal(t, ingest.SourceRejectionAssignmentMismatch, policy.Validate("system.disk.io", kubeletAttrs).Reason)
}
