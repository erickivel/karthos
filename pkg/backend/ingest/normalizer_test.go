package ingest_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/backend/ingest"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
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
	dialect := metrics.NewDialectRegistryFromSources(metrics.DefaultLogicalSources())
	return ingest.NewOTLPNormalizer(resolver, dialect)
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
	assert.Equal(t, "k8s.pod.cpu.usage", s.MetricName)
	assert.InDelta(t, 500.0, s.Value, 1e-9)
	assert.Equal(t, "otelcol", s.Labels["source"])
	assert.Equal(t, time.Unix(0, int64(tsNano)).UTC(), s.Timestamp)
}

func TestOTLPNormalizer_RawKubeletContainerCPUUsage(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC).UnixNano())
	resource := map[string]string{
		"k8s.namespace.name": "default",
		"k8s.pod.name":       "test-pod",
		"k8s.container.name": "app",
	}

	rm := makeGaugeRM("container.cpu.usage", 0.5, tsNano)
	env := makeEnvelope(t, resource, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	expectedID := entities.ComputeEntityID(entities.KindContainer, testCluster, "default", "test-pod", "app")
	assert.Equal(t, "k8s.container.cpu.usage", samples[0].MetricName)
	assert.Equal(t, expectedID, samples[0].EntityID)
	assert.InDelta(t, 500.0, samples[0].Value, 1e-9)
	assert.Equal(t, metrics.UnitMillicores, samples[0].Unit)
}

func TestOTLPNormalizer_RawKubeletContainerMemoryUsage(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC).UnixNano())
	resource := map[string]string{
		"k8s.namespace.name": "default",
		"k8s.pod.name":       "test-pod",
		"k8s.container.name": "app",
	}

	rm := makeGaugeRM("container.memory.usage", 1048576, tsNano)
	env := makeEnvelope(t, resource, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	expectedID := entities.ComputeEntityID(entities.KindContainer, testCluster, "default", "test-pod", "app")
	assert.Equal(t, "k8s.container.memory.usage", samples[0].MetricName)
	assert.Equal(t, expectedID, samples[0].EntityID)
	assert.InDelta(t, 1048576.0, samples[0].Value, 1e-9)
	assert.Equal(t, metrics.UnitBytes, samples[0].Unit)
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
	assert.Equal(t, "k8s.pod.memory.working_set", s.MetricName)
	assert.InDelta(t, 1048576.0, s.Value, 1e-9)
}

func TestOTLPNormalizer_VolumeMetricPreservesVolumeLabelsAndResolvesPVC(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRMWithAttr("k8s.volume.capacity", 1024, tsNano, "k8s.persistentvolumeclaim.name", "bundle-cache")
	env := makeEnvelope(t, podResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	s := samples[0]
	assert.Equal(t, "k8s.volume.capacity", s.MetricName)
	assert.Equal(t, entities.ComputeEntityID(entities.KindPVC, testCluster, "default", "bundle-cache"), s.EntityID)
	assert.Equal(t, "bundle-cache", s.Labels["k8s.persistentvolumeclaim.name"])
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
		assert.Equal(t, "system.disk.io.read", samples[0].MetricName)
	})

	t.Run("write direction", func(t *testing.T) {
		rm := makeGaugeRMWithAttr("system.disk.io", 2000, tsNano, "direction", "write")
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
		rm := makeGaugeRMWithAttr("system.network.io", 5000, tsNano, "direction", "transmit")
		env := makeEnvelope(t, podResource(), rm)

		samples, err := normalizer.Normalize(context.Background(), env)
		require.NoError(t, err)
		require.Len(t, samples, 1)
		assert.Equal(t, "system.network.io.transmit", samples[0].MetricName)
	})

	t.Run("receive direction", func(t *testing.T) {
		rm := makeGaugeRMWithAttr("system.network.io", 3000, tsNano, "direction", "receive")
		env := makeEnvelope(t, podResource(), rm)

		samples, err := normalizer.Normalize(context.Background(), env)
		require.NoError(t, err)
		require.Len(t, samples, 1)
		assert.Equal(t, "system.network.io.receive", samples[0].MetricName)
	})

	t.Run("tx direction", func(t *testing.T) {
		rm := makeGaugeRMWithAttr("system.network.io", 4000, tsNano, "direction", "tx")
		env := makeEnvelope(t, podResource(), rm)

		samples, err := normalizer.Normalize(context.Background(), env)
		require.NoError(t, err)
		require.Len(t, samples, 1)
		assert.Equal(t, "system.network.io.transmit", samples[0].MetricName)
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

func serviceResource() map[string]string {
	return map[string]string{
		"service.name":       "test-service",
		"k8s.namespace.name": "default",
	}
}

func TestOTLPNormalizer_ServiceGraphEdgeLabels(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRMWithAttrs("traces.service.graph.request.total", 42.0, tsNano, map[string]string{
		"client": "service-a",
		"server": "service-b",
	})
	env := makeEnvelope(t, serviceResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	s := samples[0]
	assert.Equal(t, "servicegraph.request.count", s.MetricName)
	assert.Equal(t, "service-a", s.Labels["edge_client"])
	assert.Equal(t, "service-b", s.Labels["edge_server"])
	assert.Equal(t,
		string(entities.ComputeEntityID(entities.KindService, testCluster, "default", "service-a")),
		s.Labels["edge_client_id"],
	)
	assert.Equal(t,
		string(entities.ComputeEntityID(entities.KindService, testCluster, "default", "service-b")),
		s.Labels["edge_server_id"],
	)
}

func TestOTLPNormalizer_ServiceGraphEdgeLabels_AttrsNamespace(t *testing.T) {
	// Gateway-emitted servicegraph metrics may not carry k8s.namespace.name on
	// the resource; the connector sets client_namespace/server_namespace on
	// the data point instead.
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRMWithAttrs("traces.service.graph.request.total", 7.0, tsNano, map[string]string{
		"client":           "service-a",
		"server":           "service-b",
		"client_namespace": "ns-a",
		"server_namespace": "ns-b",
	})
	// Resource resolves to a node (no namespace on envelope).
	env := makeEnvelope(t, map[string]string{"k8s.node.name": "gateway-node"}, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	s := samples[0]
	assert.Equal(t,
		string(entities.ComputeEntityID(entities.KindService, testCluster, "ns-a", "service-a")),
		s.Labels["edge_client_id"],
	)
	assert.Equal(t,
		string(entities.ComputeEntityID(entities.KindService, testCluster, "ns-b", "service-b")),
		s.Labels["edge_server_id"],
	)
}

func TestOTLPNormalizer_ServiceGraphDroppedWhenClientServiceUnresolvable(t *testing.T) {
	// When the servicegraph client service cannot be resolved to a canonical
	// Service entity (no registry match and no fallback namespace available from
	// the resource or data-point attributes), the datapoint must be dropped.
	// Falling back to the emitting resource would misattribute service traffic
	// to the collector's own node/pod.
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRMWithAttrs("traces.service.graph.request.total", 1.0, tsNano, map[string]string{
		"client": "service-a",
		"server": "service-b",
	})
	env := makeEnvelope(t, map[string]string{"k8s.node.name": "gateway-node"}, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, samples, "servicegraph datapoint with unresolvable client must be dropped")
}

func TestOTLPNormalizer_ServiceGraphDroppedWhenClientAttrMissing(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRMWithAttrs("traces.service.graph.request.total", 1.0, tsNano, map[string]string{
		"server": "service-b",
	})
	env := makeEnvelope(t, serviceResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, samples, "servicegraph datapoint without client attribute must be dropped")
}

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
	// service.name present but neither the registry nor the resource can supply
	// a namespace hint — the canonical Service entity can't be computed and the
	// datapoint must be dropped rather than mis-attributed to the emitting node.
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

func TestOTLPNormalizer_ServiceGraphUsesCanonicalServiceIDsFromRegistry(t *testing.T) {
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
	require.NoError(t, registry.UpsertEntity(context.Background(), persistence.StoredEntity{
		EntityID:   entities.ComputeEntityID(entities.KindService, testCluster, "oksg-demo", "service-a-python"),
		EntityKind: entities.KindService,
		Cluster:    testCluster,
		Namespace:  "oksg-demo",
		Name:       "service-a-python",
		Metadata: map[string]string{
			"service.name":       "service-a-python",
			"k8s.namespace.name": "oksg-demo",
		},
	}))

	normalizer := ingest.NewOTLPNormalizer(
		ingest.NewOTLPEntityResolverWithRegistry(testCluster, registry),
		metrics.NewDialectRegistry(metrics.PrometheusDialect, metrics.SpanmetricsDialect, metrics.ServicegraphDialect),
	)
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRMWithAttrs("traces.service.graph.request.total", 11.0, tsNano, map[string]string{
		"client": "gateway-go",
		"server": "service-a-python",
	})
	env := makeEnvelope(t, map[string]string{
		"k8s.node.name":       "gateway-node",
		"k8s.namespace.name":  "oksg-system",
		"k8s.cluster.name":    testCluster,
		"service.name":        "otel-collector-gateway",
		"service.instance.id": "collector-1",
	}, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	s := samples[0]
	assert.Equal(t,
		entities.ComputeEntityID(entities.KindService, testCluster, "oksg-demo", "gateway-go"),
		s.EntityID,
	)
	assert.Equal(t,
		string(entities.ComputeEntityID(entities.KindService, testCluster, "oksg-demo", "gateway-go")),
		s.Labels["edge_client_id"],
	)
	assert.Equal(t,
		string(entities.ComputeEntityID(entities.KindService, testCluster, "oksg-demo", "service-a-python")),
		s.Labels["edge_server_id"],
	)
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

func findSample(samples []metrics.MetricSample, name string) (metrics.MetricSample, bool) {
	for _, s := range samples {
		if s.MetricName == name {
			return s, true
		}
	}
	return metrics.MetricSample{}, false
}

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

func TestOTLPNormalizer_ServiceGraphServerHistogramEmitsSumCountPair(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeHistogramRMWithAttrs("traces.service.graph.request.server", 2.4, 3, tsNano, map[string]string{
		"client": "service-a",
		"server": "service-b",
	})
	env := makeEnvelope(t, serviceResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 2)

	sumSample, ok := findSample(samples, "servicegraph.request.duration_sum")
	require.True(t, ok)
	assert.InDelta(t, 2_400_000_000.0, sumSample.Value, 1e-6)
	assert.Equal(t, "service-a", sumSample.Labels["edge_client"])
	assert.Equal(t, "service-b", sumSample.Labels["edge_server"])

	countSample, ok := findSample(samples, "servicegraph.request.duration_count")
	require.True(t, ok)
	assert.InDelta(t, 3.0, countSample.Value, 1e-9)
	assert.Equal(t, "service-a", countSample.Labels["edge_client"])
}

func TestOTLPNormalizer_ServiceGraphFailedTotalMapsToErrorRateCounter(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRMWithAttrs("traces.service.graph.request_failed_total", 9, tsNano, map[string]string{
		"client": "service-a",
		"server": "service-b",
	})
	env := makeEnvelope(t, serviceResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	assert.Equal(t, "servicegraph.request.error.count", samples[0].MetricName)
	assert.Equal(t, "service-a", samples[0].Labels["edge_client"])
	assert.Equal(t, "service-b", samples[0].Labels["edge_server"])
}

func TestOTLPNormalizer_IstioRequestsUseServicegraphEdgeLabelsAndMeshMetadata(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRMWithAttrs("istio_requests_total", 42.0, tsNano, map[string]string{
		"client":                          "gateway-go",
		"server":                          "service-a-python",
		"client_namespace":                "oksg-demo",
		"server_namespace":                "oksg-demo",
		"network.protocol.name":           "http",
		"http.response.status_code":       "200",
		"mesh.security.policy":            "mutual_tls",
		"mesh.peer.source.principal":      "spiffe://cluster.local/ns/oksg-demo/sa/gateway",
		"mesh.peer.destination.principal": "spiffe://cluster.local/ns/oksg-demo/sa/service-a",
		"oksg.source":                     "mesh.istio",
	})
	env := makeEnvelope(t, map[string]string{"k8s.node.name": "worker-1"}, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	s := samples[0]
	assert.Equal(t, "servicegraph.request.count", s.MetricName)
	assert.Equal(t, metrics.UnitPerSecond, s.Unit)
	assert.Equal(t, "mesh.istio", s.Labels["source"])
	assert.Equal(t, "gateway-go", s.Labels["edge_client"])
	assert.Equal(t, "service-a-python", s.Labels["edge_server"])
	assert.Equal(t, "http", s.Labels["network.protocol.name"])
	assert.Equal(t, "mutual_tls", s.Labels["mesh.security.policy"])
	assert.Equal(t,
		string(entities.ComputeEntityID(entities.KindService, testCluster, "oksg-demo", "gateway-go")),
		s.Labels["edge_client_id"],
	)
	assert.Equal(t,
		string(entities.ComputeEntityID(entities.KindService, testCluster, "oksg-demo", "service-a-python")),
		s.Labels["edge_server_id"],
	)
}

func TestOTLPNormalizer_Istio5xxRequestsAlsoEmitErrorCount(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRMWithAttrs("istio_requests_total", 3.0, tsNano, map[string]string{
		"client":                    "gateway-go",
		"server":                    "service-a-python",
		"client_namespace":          "oksg-demo",
		"server_namespace":          "oksg-demo",
		"http.response.status_code": "503",
		"oksg.source":               "mesh.istio",
	})
	env := makeEnvelope(t, map[string]string{"k8s.node.name": "worker-1"}, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 2)

	_, ok := findSample(samples, "servicegraph.request.count")
	require.True(t, ok)
	errSample, ok := findSample(samples, "servicegraph.request.error.count")
	require.True(t, ok)
	assert.Equal(t, "mesh.istio", errSample.Labels["source"])
	assert.Equal(t, "503", errSample.Labels["http.response.status_code"])
}

func TestOTLPNormalizer_IstioDurationMillisecondsHistogramEmitsNanosecondSumCount(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeHistogramRMWithAttrs("istio_request_duration_milliseconds", 12.5, 5, tsNano, map[string]string{
		"client":           "gateway-go",
		"server":           "service-a-python",
		"client_namespace": "oksg-demo",
		"server_namespace": "oksg-demo",
		"oksg.source":      "mesh.istio",
	})
	env := makeEnvelope(t, map[string]string{"k8s.node.name": "worker-1"}, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 2)

	sumSample, ok := findSample(samples, "servicegraph.request.duration_sum")
	require.True(t, ok)
	assert.InDelta(t, 12_500_000.0, sumSample.Value, 1e-6)
	assert.Equal(t, metrics.UnitNanoseconds, sumSample.Unit)

	countSample, ok := findSample(samples, "servicegraph.request.duration_count")
	require.True(t, ok)
	assert.InDelta(t, 5.0, countSample.Value, 1e-9)
}

func TestOTLPNormalizer_HistogramWithZeroCountSkipped(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeHistogramRM("http_request_duration_seconds", 1.5, 0, tsNano)
	env := makeEnvelope(t, podResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, samples)
}

func TestOTLPNormalizer_NonServiceGraphHasNoEdgeLabels(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRM("k8s.pod.cpu.usage", 0.5, tsNano)
	env := makeEnvelope(t, podResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	_, hasClient := samples[0].Labels["edge_client"]
	_, hasServer := samples[0].Labels["edge_server"]
	assert.False(t, hasClient, "non-servicegraph metric should not have edge_client label")
	assert.False(t, hasServer, "non-servicegraph metric should not have edge_server label")
}

func TestOTLPNormalizer_NodeConditionReady(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())
	nodeResource := map[string]string{"k8s.node.name": "test-node"}

	rm := makeGaugeRMWithAttr("k8s.node.condition", 1.0, tsNano, "condition", "Ready")
	env := makeEnvelope(t, nodeResource, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)
	assert.Equal(t, "k8s.node.condition.ready", samples[0].MetricName)
}

func TestOTLPNormalizer_NodeConditionMemoryPressure(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())
	nodeResource := map[string]string{"k8s.node.name": "test-node"}

	rm := makeGaugeRMWithAttr("k8s.node.condition", 0.0, tsNano, "condition", "MemoryPressure")
	env := makeEnvelope(t, nodeResource, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)
	assert.Equal(t, "k8s.node.condition.memory_pressure", samples[0].MetricName)
}

func TestOTLPNormalizer_NodeConditionUnknownSkipped(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())
	nodeResource := map[string]string{"k8s.node.name": "test-node"}

	rm := makeGaugeRMWithAttr("k8s.node.condition", 0.0, tsNano, "condition", "SomeUnknownCondition")
	env := makeEnvelope(t, nodeResource, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, samples, "unknown condition type should be skipped")
}

func TestOTLPNormalizer_DeploymentEntityResolution(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())
	deployResource := map[string]string{
		"k8s.deployment.name": "my-deployment",
		"k8s.namespace.name":  "default",
	}

	rm := makeGaugeRM("k8s.deployment.desired", 3.0, tsNano)
	env := makeEnvelope(t, deployResource, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)
	assert.Equal(t, "k8s.deployment.desired", samples[0].MetricName)
	assert.InDelta(t, 3.0, samples[0].Value, 1e-9)
}

func TestOTLPNormalizer_SumDataPointInt64(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	// Use k8s.replicaset.desired which now stays semconv-native.
	rm := makeSumRMInt("k8s.replicaset.desired", 3, tsNano)
	env := makeEnvelope(t, podResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	s := samples[0]
	assert.Equal(t, "k8s.replicaset.desired", s.MetricName)
	assert.InDelta(t, 3.0, s.Value, 1e-9)
}

// TestOTLPNormalizer_SourceMismatchDropped verifies that when an Assignment is
// configured, samples whose oksg_source disagrees with the assigned source are
// dropped and counted, while matching samples pass through.
func TestOTLPNormalizer_SourceMismatchDropped(t *testing.T) {
	resolver := ingest.NewOTLPEntityResolver(testCluster)
	dialect := metrics.NewDialectRegistry(
		metrics.PrometheusDialect,
		metrics.SpanmetricsDialect,
		metrics.ServicegraphDialect,
		metrics.IstioDialect,
	)
	// Using k8s.pod.cpu.usage as the test vehicle: it lives in OTLPSemconvMetrics
	// so the normalizer's translation lookup succeeds. Assignment pins it to
	// mesh.istio (synthetic — kubelet is the real catalog winner) so we can
	// observe the kubelet-tagged sample being dropped.
	assignment := metrics.Assignment{
		"k8s.pod.cpu.usage": metrics.SourceMeshIstio,
	}
	normalizer := ingest.NewOTLPNormalizer(resolver, dialect, ingest.WithAssignment(assignment))

	tsNano := uint64(time.Now().UnixNano())

	// Sample tagged with the wrong source: should be dropped.
	rmKubelet := makeGaugeRMWithAttr("k8s.pod.cpu.usage", 0.5, tsNano, "oksg_source", metrics.SourceKubelet)
	envKubelet := makeEnvelope(t, podResource(), rmKubelet)
	samples, err := normalizer.Normalize(context.Background(), envKubelet)
	require.NoError(t, err)
	require.Empty(t, samples, "kubelet sample must be dropped when istio is the assigned source")
	require.Equal(t, uint64(1), normalizer.DroppedSampleCount())

	// Sample tagged with the matching source: should pass through.
	rmIstio := makeGaugeRMWithAttr("k8s.pod.cpu.usage", 0.75, tsNano, "oksg_source", metrics.SourceMeshIstio)
	envIstio := makeEnvelope(t, podResource(), rmIstio)
	samples, err = normalizer.Normalize(context.Background(), envIstio)
	require.NoError(t, err)
	require.Len(t, samples, 1)
	require.InDelta(t, 750.0, samples[0].Value, 1e-9)
	require.Equal(t, uint64(1), normalizer.DroppedSampleCount())

	// Sample without an oksg_source attribute: legacy path, accepted.
	rmUntagged := makeGaugeRM("k8s.pod.cpu.usage", 1.0, tsNano)
	envUntagged := makeEnvelope(t, podResource(), rmUntagged)
	samples, err = normalizer.Normalize(context.Background(), envUntagged)
	require.NoError(t, err)
	require.Len(t, samples, 1, "untagged samples must be accepted (legacy / unknown-source path)")
}
