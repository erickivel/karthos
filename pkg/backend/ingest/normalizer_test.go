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
	assert.Equal(t, "request_rate_per_second", s.MetricName)
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

func TestOTLPNormalizer_ServiceGraphEdgeLabels_NoNamespace(t *testing.T) {
	// If neither dp attribute nor envelope resource supplies a namespace,
	// the edge_*_id labels are omitted rather than computed with an empty ns.
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRMWithAttrs("traces.service.graph.request.total", 1.0, tsNano, map[string]string{
		"client": "service-a",
		"server": "service-b",
	})
	env := makeEnvelope(t, map[string]string{"k8s.node.name": "gateway-node"}, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	s := samples[0]
	assert.Equal(t, "service-a", s.Labels["edge_client"])
	assert.Equal(t, "service-b", s.Labels["edge_server"])
	_, hasClientID := s.Labels["edge_client_id"]
	_, hasServerID := s.Labels["edge_server_id"]
	assert.False(t, hasClientID, "edge_client_id should be omitted without namespace")
	assert.False(t, hasServerID, "edge_server_id should be omitted without namespace")
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

	normalizer := ingest.NewOTLPNormalizer(ingest.NewOTLPEntityResolverWithRegistry(testCluster, registry))
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

	normalizer := ingest.NewOTLPNormalizer(ingest.NewOTLPEntityResolverWithRegistry(testCluster, registry))
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
	assert.Equal(t, "node_condition_ready", samples[0].MetricName)
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
	assert.Equal(t, "node_condition_memory_pressure", samples[0].MetricName)
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
	assert.Equal(t, "replica_count", samples[0].MetricName)
	assert.InDelta(t, 3.0, samples[0].Value, 1e-9)
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
