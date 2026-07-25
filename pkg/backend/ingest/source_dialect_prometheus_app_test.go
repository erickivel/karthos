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

func TestOTLPNormalizer_PrometheusDurationHistogramEmitsSumCountPair(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeHistogramRM("http_request_duration_seconds", 1.5, 3, tsNano)
	env := makeEnvelope(t, podResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 2)

	sumSample, ok := findSample(samples, "http.server.request.duration_sum")
	require.True(t, ok, "expected _sum sample")
	assert.InDelta(t, 1_500_000_000.0, sumSample.Value, 1e-6)

	countSample, ok := findSample(samples, "http.server.request.duration_count")
	require.True(t, ok, "expected _count sample")
	assert.InDelta(t, 3.0, countSample.Value, 1e-9)
}

func TestOTLPNormalizer_PrometheusAppMetricResolvesServiceAndPreservesBoundedLabels(t *testing.T) {
	registry := persistence.NewInMemoryEntityRegistry()
	serviceID := entities.ComputeEntityID(entities.KindService, testCluster, "oksg-demo", "gateway-go")
	require.NoError(t, registry.UpsertEntity(context.Background(), persistence.StoredEntity{
		EntityID:   serviceID,
		EntityKind: entities.KindService,
		Cluster:    testCluster,
		Namespace:  "oksg-demo",
		Name:       "gateway-go",
		Metadata:   map[string]string{"name": "gateway-go", "namespace": "oksg-demo"},
	}))

	normalizer := ingest.NewOTLPNormalizer(
		ingest.NewOTLPEntityResolverWithRegistry(testCluster, registry),
		metrics.NewDialectRegistry(metrics.PrometheusDialect),
	)
	tsNano := uint64(time.Now().UnixNano())
	rm := makeSumRMIntWithAttrs("http_requests_total", 7, tsNano, map[string]string{
		"oksg_source":  metrics.SourcePrometheusApp,
		"namespace":    "oksg-demo",
		"service":      "gateway-go",
		"pod":          "gateway-go-abc123",
		"instance":     "10.42.0.11:8080",
		"metrics_path": "/metrics",
		"service_port": "8080",
		"method":       "GET",
		"path":         "/orders/7c4d9d7e-6dc5-49d4-a660-d8d5b8475b72",
		"http.route":   "/orders/{order_id}",
		"status":       "500",
	}, metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, true)
	env := makeEnvelope(t, map[string]string{
		"k8s.node.name":    "worker-1",
		"k8s.cluster.name": testCluster,
	}, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	s := samples[0]
	assert.Equal(t, "http.server.request.count", s.MetricName)
	assert.Equal(t, metrics.UnitCount, s.Unit)
	assert.Equal(t, serviceID, s.EntityID)
	assert.Equal(t, metrics.SourcePrometheusApp, s.Labels["source"])
	assert.Equal(t, "oksg-demo", s.Labels["namespace"])
	assert.Equal(t, "gateway-go", s.Labels["service"])
	assert.Equal(t, "gateway-go-abc123", s.Labels["pod"])
	assert.Equal(t, "10.42.0.11:8080", s.Labels["instance"])
	assert.Equal(t, "/metrics", s.Labels["metrics_path"])
	assert.Equal(t, "8080", s.Labels["service_port"])
	assert.Equal(t, "GET", s.Labels["method"])
	assert.Equal(t, "/orders/{order_id}", s.Labels["http_route"])
	assert.Equal(t, "500", s.Labels["status"])
	assert.NotContains(t, s.Labels, "path")
}

func TestOTLPNormalizer_PrometheusDurationPreservesSameLabelsOnSumAndCount(t *testing.T) {
	registry := persistence.NewInMemoryEntityRegistry()
	serviceID := entities.ComputeEntityID(entities.KindService, testCluster, "oksg-demo", "gateway-go")
	require.NoError(t, registry.UpsertEntity(context.Background(), persistence.StoredEntity{
		EntityID:   serviceID,
		EntityKind: entities.KindService,
		Cluster:    testCluster,
		Namespace:  "oksg-demo",
		Name:       "gateway-go",
		Metadata:   map[string]string{"name": "gateway-go", "namespace": "oksg-demo"},
	}))
	normalizer := ingest.NewOTLPNormalizer(
		ingest.NewOTLPEntityResolverWithRegistry(testCluster, registry),
		metrics.NewDialectRegistry(metrics.PrometheusDialect),
	)
	tsNano := uint64(time.Now().UnixNano())
	attrs := map[string]string{
		"oksg_source":  metrics.SourcePrometheusApp,
		"namespace":    "oksg-demo",
		"service":      "gateway-go",
		"pod":          "gateway-go-abc123",
		"metrics_path": "/metrics",
		"service_port": "8080",
		"method":       "GET",
		"path":         "/orders/7c4d9d7e-6dc5-49d4-a660-d8d5b8475b72",
		"http.route":   "/orders/{order_id}",
	}
	rm := makeHistogramRMWithAttrs("http_request_duration_seconds", 1.5, 3, tsNano, attrs)
	env := makeEnvelope(t, map[string]string{"k8s.node.name": "worker-1"}, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 2)

	sumSample, ok := findSample(samples, "http.server.request.duration_sum")
	require.True(t, ok)
	countSample, ok := findSample(samples, "http.server.request.duration_count")
	require.True(t, ok)
	assert.Equal(t, serviceID, sumSample.EntityID)
	assert.Equal(t, sumSample.Labels, countSample.Labels)
	assert.Equal(t, "gateway-go-abc123", sumSample.Labels["pod"])
	assert.Equal(t, "GET", sumSample.Labels["method"])
	assert.Equal(t, "/orders/{order_id}", sumSample.Labels["http_route"])
	assert.NotContains(t, sumSample.Labels, "path")
	assert.Equal(t, "/metrics", sumSample.Labels["metrics_path"])
	assert.Equal(t, "8080", sumSample.Labels["service_port"])
}
