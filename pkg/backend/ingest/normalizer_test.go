package ingest_test

import (
	"context"
	"encoding/json"
	"errors"
	"testing"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/backend/ingest"
	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/sources"
)

const testCluster = "test-cluster"

type failingEntityRegistry struct {
	persistence.EntityRegistry
	err error
}

func (r failingEntityRegistry) UpsertEntity(context.Context, entities.Entity) error {
	return r.err
}

func marshalPayload(rm *metricspb.ResourceMetrics) json.RawMessage {
	b, _ := protojson.Marshal(rm)
	return json.RawMessage(b)
}

func makeEnvelope(t *testing.T, source string, resource map[string]string, rm *metricspb.ResourceMetrics) schema.RawEnvelope {
	t.Helper()
	stampMetricSource(rm, source)
	return makeEnvelopeWithoutSourceStamp(t, resource, rm)
}

func makeEnvelopeWithoutSourceStamp(t *testing.T, resource map[string]string, rm *metricspb.ResourceMetrics) schema.RawEnvelope {
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

func stampMetricSource(rm *metricspb.ResourceMetrics, source string) {
	for _, scope := range rm.ScopeMetrics {
		for _, metric := range scope.Metrics {
			for _, dp := range metricAttributeSetsForTest(metric) {
				if testAttr(dp, "oksg.source") != "" || testAttr(dp, "oksg_source") != "" {
					continue
				}
				*dp = append(*dp, &commonpb.KeyValue{Key: "oksg.source", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: source}}})
			}
		}
	}
}

func metricAttributeSetsForTest(metric *metricspb.Metric) []*[]*commonpb.KeyValue {
	var sets []*[]*commonpb.KeyValue
	if gauge := metric.GetGauge(); gauge != nil {
		for _, dp := range gauge.DataPoints {
			sets = append(sets, &dp.Attributes)
		}
	}
	if sum := metric.GetSum(); sum != nil {
		for _, dp := range sum.DataPoints {
			sets = append(sets, &dp.Attributes)
		}
	}
	if histogram := metric.GetHistogram(); histogram != nil {
		for _, dp := range histogram.DataPoints {
			sets = append(sets, &dp.Attributes)
		}
	}
	return sets
}

func testAttr(attrs *[]*commonpb.KeyValue, key string) string {
	for _, attr := range *attrs {
		if attr.Key == key && attr.Value != nil {
			return attr.Value.GetStringValue()
		}
	}
	return ""
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

func makeHistogramRM(metricName string, sum float64, count uint64, tsNano uint64) *metricspb.ResourceMetrics {
	return makeHistogramRMWithAttrs(metricName, sum, count, tsNano, nil)
}

func makeHistogramRMWithAttrs(
	metricName string,
	sum float64,
	count uint64,
	tsNano uint64,
	attrs map[string]string,
) *metricspb.ResourceMetrics {
	var kvs []*commonpb.KeyValue
	for k, v := range attrs {
		kvs = append(kvs, &commonpb.KeyValue{
			Key:   k,
			Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}},
		})
	}

	return &metricspb.ResourceMetrics{
		ScopeMetrics: []*metricspb.ScopeMetrics{
			{
				Metrics: []*metricspb.Metric{
					{
						Name: metricName,
						Data: &metricspb.Metric_Histogram{
							Histogram: &metricspb.Histogram{
								DataPoints: []*metricspb.HistogramDataPoint{
									{
										Attributes:   kvs,
										Sum:          &sum,
										Count:        count,
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
	dialect := metrics.NewSourceDialectRegistry(sources.DefaultRegistry().Dialects())
	return ingest.NewOTLPNormalizer(resolver, dialect)
}

func TestOTLPNormalizer_MemoryMetric(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRM("k8s.pod.memory.working_set", 1048576, tsNano)
	env := makeEnvelope(t, metrics.SourceKubelet, podResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	s := samples[0]
	assert.Equal(t, "k8s.pod.memory.working_set", s.MetricName)
	assert.InDelta(t, 1048576.0, s.Value, 1e-9)
}

func TestOTLPNormalizer_UnmappedMetricSkipped(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRM("some.unknown.metric", 42.0, tsNano)
	env := makeEnvelope(t, metrics.SourceKubelet, podResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, samples)
}

func TestOTLPNormalizer_EntityResolutionError(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRM("k8s.pod.cpu.usage", 0.5, tsNano)

	env := makeEnvelope(t, metrics.SourceKubelet, map[string]string{"some.unknown.attr": "value"}, rm)

	_, err := normalizer.Normalize(context.Background(), env)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "resolve entity")
}

func TestNormalizingIngestor_RejectsMetricWhenEntityPersistenceFails(t *testing.T) {
	entityErr := errors.New("entity registry unavailable")
	resolver := ingest.NewOTLPEntityResolverWithRegistry(testCluster, failingEntityRegistry{err: entityErr})
	normalizer := ingest.NewOTLPNormalizer(resolver, metrics.NewSourceDialectRegistry(sources.DefaultRegistry().Dialects()))
	store := persistence.NewInMemoryMetricStore()
	ingestor := ingest.NewNormalizingIngestor(normalizer, store, nil)
	env := makeEnvelope(t, metrics.SourceKubelet, podResource(), makeGaugeRM("k8s.pod.cpu.usage", 0.5, uint64(time.Now().UnixNano())))

	err := ingestor.Ingest(context.Background(), env)
	require.ErrorIs(t, err, entityErr)

	samples, err := store.Query(context.Background(), query.MetricQuery{})
	require.NoError(t, err)
	assert.Empty(t, samples)
}

func makeGaugeRMWithAttrs(metricName string, value float64, tsNano uint64, attrs map[string]string) *metricspb.ResourceMetrics {
	var kvs []*commonpb.KeyValue
	for k, v := range attrs {
		kvs = append(kvs, &commonpb.KeyValue{
			Key:   k,
			Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}},
		})
	}
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
										Attributes:   kvs,
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

func TestOTLPNormalizer_HistogramWithZeroCountSkipped(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeHistogramRM("http_request_duration_seconds", 1.5, 0, tsNano)
	env := makeEnvelope(t, metrics.SourcePrometheusApp, podResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, samples)
}

func TestOTLPNormalizer_DropsScalarDataPointWithoutTimestamp(t *testing.T) {
	normalizer := newNormalizer()
	env := makeEnvelope(t, metrics.SourceKubelet, podResource(), makeGaugeRM("k8s.pod.cpu.usage", 0.5, 0))

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, samples)
}

func TestOTLPNormalizer_DropsHistogramDataPointWithoutTimestamp(t *testing.T) {
	normalizer := newNormalizer()
	env := makeEnvelope(t, metrics.SourcePrometheusApp, podResource(), makeHistogramRM("http_request_duration_seconds", 1.5, 3, 0))

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, samples)
}

// TestOTLPNormalizer_SourceMismatchDropped verifies that assignment enforcement
// retains only samples stamped by the assigned source.
func TestOTLPNormalizer_SourceMismatchDropped(t *testing.T) {
	resolver := ingest.NewOTLPEntityResolver(testCluster)
	dialect := metrics.NewSourceDialectRegistry(sources.DefaultRegistry().Dialects())

	assignment := metrics.Assignment{
		"k8s.pod.cpu.usage": metrics.SourceMeshIstio,
	}
	policy := ingest.NewMetricSourcePolicy(sources.DefaultRegistry().MetricIDs(), assignment)
	normalizer := ingest.NewOTLPNormalizer(resolver, dialect, ingest.WithSourcePolicy(policy))

	tsNano := uint64(time.Now().UnixNano())

	rmKubelet := makeGaugeRMWithAttr("k8s.pod.cpu.usage", 0.5, tsNano, "oksg_source", metrics.SourceKubelet)
	envKubelet := makeEnvelope(t, metrics.SourceKubelet, podResource(), rmKubelet)
	samples, err := normalizer.Normalize(context.Background(), envKubelet)
	require.NoError(t, err)
	require.Empty(t, samples, "kubelet sample must be dropped when istio is the assigned source")

	rmIstio := makeGaugeRMWithAttr("k8s.pod.cpu.usage", 0.75, tsNano, "oksg_source", metrics.SourceMeshIstio)
	envIstio := makeEnvelope(t, metrics.SourceMeshIstio, podResource(), rmIstio)
	samples, err = normalizer.Normalize(context.Background(), envIstio)
	require.NoError(t, err)
	require.Len(t, samples, 1)
	require.InDelta(t, 750.0, samples[0].Value, 1e-9)

	rmUntagged := makeGaugeRM("k8s.pod.cpu.usage", 1.0, tsNano)
	envUntagged := makeEnvelopeWithoutSourceStamp(t, podResource(), rmUntagged)
	samples, err = normalizer.Normalize(context.Background(), envUntagged)
	require.NoError(t, err)
	require.Empty(t, samples, "recognized untagged samples must be rejected")
}
