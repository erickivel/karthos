package ingest_test

import (
	"context"

	"testing"
	"time"

	"github.com/erickivel/oksg/pkg/backend/persistence"

	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/backend/ingest"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
)

func TestOTLPNormalizer_SpanMetricsDroppedWhenServiceNameMissing(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRM("traces.span.metrics.calls", 5.0, tsNano)
	env := makeEnvelope(t, map[string]string{"k8s.node.name": "worker-1"}, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, samples, "spanmetrics datapoint without service.name must be dropped")
}

func TestOTLPNormalizer_SpanMetricsDroppedWhenNamespaceUnknown(t *testing.T) {

	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRM("traces.span.metrics.calls", 5.0, tsNano)
	env := makeEnvelope(t, map[string]string{
		"service.name":  "some-service",
		"k8s.node.name": "worker-1",
	}, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, samples, "spanmetrics datapoint without resolvable namespace must be dropped")
}

func TestOTLPNormalizer_SpanMetricsUseServiceEntityOverNode(t *testing.T) {
	registry := persistence.NewInMemoryEntityRegistry()
	require.NoError(t, registry.UpsertEntity(context.Background(), persistence.StoredEntity{
		EntityID:   entities.ComputeEntityID(entities.KindService, testCluster, "oksg-demo", "gateway-go"),
		EntityKind: entities.KindService,
		Cluster:    testCluster,
		Namespace:  "oksg-demo",
		Name:       "gateway-go",
		Metadata: map[string]string{
			"service.name":       "gateway-go",
			"k8s.namespace.name": "oksg-demo",
		},
	}))

	normalizer := ingest.NewOTLPNormalizer(
		ingest.NewOTLPEntityResolverWithRegistry(testCluster, registry),
		metrics.NewDialectRegistry(metrics.PrometheusDialect, metrics.SpanmetricsDialect, metrics.ServicegraphDialect),
	)
	tsNano := uint64(time.Now().UnixNano())
	rm := makeGaugeRM("traces.span.metrics.calls", 5.0, tsNano)
	env := makeEnvelope(t, map[string]string{
		"service.name":       "gateway-go",
		"k8s.namespace.name": "oksg-demo",
		"k8s.node.name":      "worker-1",
	}, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	assert.Equal(t,
		entities.ComputeEntityID(entities.KindService, testCluster, "oksg-demo", "gateway-go"),
		samples[0].EntityID,
	)
}

func TestOTLPNormalizer_SpanMetricsDurationHistogramEmitsSumCountPair(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeHistogramRM("traces.span.metrics.duration", 18, 3, tsNano)
	env := makeEnvelope(t, serviceResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 2)

	sumSample, ok := findSample(samples, "rpc.server.duration_sum")
	require.True(t, ok)
	assert.InDelta(t, 18_000_000.0, sumSample.Value, 1e-6)

	countSample, ok := findSample(samples, "rpc.server.duration_count")
	require.True(t, ok)
	assert.InDelta(t, 3.0, countSample.Value, 1e-9)
}

func TestOTLPNormalizer_SpanMetricsPreservesBoundedDimensions(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())
	rm := makeSumRMIntWithAttrs("traces.span.metrics.calls", 5, tsNano, map[string]string{
		"oksg_source": metrics.SourceTracesSpanmetrics,
		"http.method": "POST",
		"span.name":   "POST /orders/7c4d9d7e-6dc5-49d4-a660-d8d5b8475b72",
		"span_name":   "POST /orders/7c4d9d7e-6dc5-49d4-a660-d8d5b8475b72",
		"span.kind":   "server",
		"status.code": "ERROR",
	}, metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, true)
	appendIntAttr(&rm.ScopeMetrics[0].Metrics[0].GetSum().DataPoints[0].Attributes, "http.response.status_code", 500)
	env := makeEnvelope(t, serviceResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	assert.Equal(t, "POST", samples[0].Labels["http_method"])
	assert.Equal(t, metrics.UnitCount, samples[0].Unit)
	assert.Equal(t, "500", samples[0].Labels["http_status_code"])
	assert.Equal(t, "server", samples[0].Labels["span_kind"])
	assert.Equal(t, "ERROR", samples[0].Labels["status_code"])
	assert.NotContains(t, samples[0].Labels, "span_name")
	assert.NotContains(t, samples[0].Labels, "span.name")
}

func TestOTLPNormalizer_DeltaSumsRemainRawBySeries(t *testing.T) {
	normalizer := newNormalizer()
	now := time.Now()
	attrs := map[string]string{
		"oksg_source": metrics.SourceTracesSpanmetrics,
		"http.method": "GET",
	}
	env1 := makeEnvelope(t, serviceResource(), makeSumRMIntWithAttrs(
		"traces.span.metrics.calls",
		2,
		uint64(now.UnixNano()),
		attrs,
		metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
		true,
	))
	env2 := makeEnvelope(t, serviceResource(), makeSumRMIntWithAttrs(
		"traces.span.metrics.calls",
		3,
		uint64(now.Add(15*time.Second).UnixNano()),
		attrs,
		metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
		true,
	))

	first, err := normalizer.Normalize(context.Background(), env1)
	require.NoError(t, err)
	second, err := normalizer.Normalize(context.Background(), env2)
	require.NoError(t, err)

	require.Len(t, first, 1)
	require.Len(t, second, 1)
	assert.Equal(t, 2.0, first[0].Value)
	assert.Equal(t, 3.0, second[0].Value)
	assert.Equal(t, "delta", second[0].Labels["temporality"])
}
