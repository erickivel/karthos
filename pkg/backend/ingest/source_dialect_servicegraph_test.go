package ingest_test

import (
	"context"
	"sync"

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

type countingEntityRegistry struct {
	delegate  persistence.EntityRegistry
	mu        sync.Mutex
	listCalls int
}

func (r *countingEntityRegistry) UpsertEntity(ctx context.Context, entity entities.Entity) error {
	return r.delegate.UpsertEntity(ctx, entity)
}

func (r *countingEntityRegistry) GetEntity(ctx context.Context, id entities.EntityID) (entities.Entity, error) {
	return r.delegate.GetEntity(ctx, id)
}

func (r *countingEntityRegistry) GetEntities(ctx context.Context, ids []entities.EntityID) (map[entities.EntityID]entities.Entity, error) {
	return r.delegate.GetEntities(ctx, ids)
}

func (r *countingEntityRegistry) ListEntities(ctx context.Context, query persistence.EntityQuery) ([]entities.Entity, error) {
	r.mu.Lock()
	r.listCalls++
	r.mu.Unlock()
	return r.delegate.ListEntities(ctx, query)
}

func (r *countingEntityRegistry) count() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.listCalls
}

func TestOTLPNormalizer_ServiceGraphResolvesEachPeerOnce(t *testing.T) {
	registry := &countingEntityRegistry{delegate: persistence.NewInMemoryEntityRegistry()}
	for _, name := range []string{"service-a", "service-b"} {
		require.NoError(t, registry.UpsertEntity(context.Background(), persistence.StoredEntity{
			EntityID:   entities.ComputeEntityID(entities.KindService, testCluster, "default", name),
			EntityKind: entities.KindService,
			Cluster:    testCluster,
			Namespace:  "default",
			Name:       name,
		}))
	}
	normalizer := ingest.NewOTLPNormalizer(
		ingest.NewOTLPEntityResolverWithRegistry(testCluster, registry),
		metrics.NewDialectRegistryFromSources(metrics.DefaultLogicalSources()),
	)
	rm := makeGaugeRMWithAttrs("traces_service_graph_request_total", 1, uint64(time.Now().UnixNano()), map[string]string{
		"client": "service-a",
		"server": "service-b",
	})

	samples, err := normalizer.Normalize(context.Background(), makeEnvelope(t, serviceResource(), rm))
	require.NoError(t, err)
	require.Len(t, samples, 1)
	assert.Equal(t, 2, registry.count(), "client identity must be reused for labeling and the required-client gate")
}

func TestOTLPNormalizer_ServiceGraphEdgeLabels(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRMWithAttrs("traces_service_graph_request_total", 42.0, tsNano, map[string]string{
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

	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRMWithAttrs("traces_service_graph_request_total", 7.0, tsNano, map[string]string{
		"client":           "service-a",
		"server":           "service-b",
		"client_namespace": "ns-a",
		"server_namespace": "ns-b",
	})

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

	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRMWithAttrs("traces_service_graph_request_total", 1.0, tsNano, map[string]string{
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

	rm := makeGaugeRMWithAttrs("traces_service_graph_request_total", 1.0, tsNano, map[string]string{
		"server": "service-b",
	})
	env := makeEnvelope(t, serviceResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, samples, "servicegraph datapoint without client attribute must be dropped")
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

	rm := makeGaugeRMWithAttrs("traces_service_graph_request_total", 11.0, tsNano, map[string]string{
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

func TestOTLPNormalizer_ServiceGraphServerHistogramEmitsSumCountPair(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeHistogramRMWithAttrs("traces_service_graph_request_server", 2.4, 3, tsNano, map[string]string{
		"client":             "service-a",
		"server":             "service-b",
		"connection_type":    "database",
		"virtual_node":       "server",
		"client_http.method": "POST",
		"server_http.method": "GET",
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
	assert.Equal(t, sumSample.Labels, countSample.Labels)
	assert.Equal(t, "database", sumSample.Labels["connection_type"])
	assert.Equal(t, "server", sumSample.Labels["virtual_node"])
	assert.Equal(t, "POST", sumSample.Labels["client_http_method"])
	assert.Equal(t, "GET", sumSample.Labels["server_http_method"])
}

func TestOTLPNormalizer_ServiceGraphFailedTotalMapsToErrorRateCounter(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRMWithAttrs("traces_service_graph_request_failed_total", 9, tsNano, map[string]string{
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

func TestOTLPNormalizer_ServiceGraphPreservesEdgeDimensions(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeSumRMIntWithAttrs("traces_service_graph_request_total", 9, tsNano, map[string]string{
		"oksg_source":               metrics.SourceTracesServicegraph,
		"client":                    "service-a",
		"server":                    "service-b",
		"client_namespace":          "default",
		"server_namespace":          "default",
		"http.method":               "GET",
		"http.response.status_code": "200",
		"connection_type":           "messaging_system",
		"virtual_node":              "client",
		"client_http.method":        "POST",
		"server_http.method":        "GET",
	}, metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, true)
	appendBoolAttr(&rm.ScopeMetrics[0].Metrics[0].GetSum().DataPoints[0].Attributes, "failed", true)
	env := makeEnvelope(t, serviceResource(), rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 1)

	assert.Equal(t, "service-a", samples[0].Labels["edge_client"])
	assert.Equal(t, "service-b", samples[0].Labels["edge_server"])
	assert.Equal(t, "default", samples[0].Labels["client_namespace"])
	assert.Equal(t, "default", samples[0].Labels["server_namespace"])
	assert.Equal(t, "GET", samples[0].Labels["http_method"])
	assert.Equal(t, "200", samples[0].Labels["http_status_code"])
	assert.Equal(t, "true", samples[0].Labels["failed"])
	assert.Equal(t, "messaging_system", samples[0].Labels["connection_type"])
	assert.Equal(t, "client", samples[0].Labels["virtual_node"])
	assert.Equal(t, "POST", samples[0].Labels["client_http_method"])
	assert.Equal(t, "GET", samples[0].Labels["server_http_method"])
}

func TestOTLPNormalizer_ServiceGraphRequestContextsRemainSeparate(t *testing.T) {
	normalizer := newNormalizer()
	now := time.Now()
	base := map[string]string{
		"oksg_source":      metrics.SourceTracesServicegraph,
		"client":           "service-a",
		"server":           "service-b",
		"client_namespace": "default",
		"server_namespace": "default",
		"connection_type":  "database",
	}
	getAttrs := make(map[string]string, len(base)+2)
	postAttrs := make(map[string]string, len(base)+2)
	for key, value := range base {
		getAttrs[key] = value
		postAttrs[key] = value
	}
	getAttrs["client_http.method"] = "GET"
	getAttrs["server_http.method"] = "GET"
	postAttrs["client_http.method"] = "POST"
	postAttrs["server_http.method"] = "POST"

	getSamples, err := normalizer.Normalize(context.Background(), makeEnvelope(t, serviceResource(), makeSumRMIntWithAttrs(
		"traces_service_graph_request_total", 4, uint64(now.UnixNano()), getAttrs,
		metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, true,
	)))
	require.NoError(t, err)
	postSamples, err := normalizer.Normalize(context.Background(), makeEnvelope(t, serviceResource(), makeSumRMIntWithAttrs(
		"traces_service_graph_request_total", 7, uint64(now.Add(time.Second).UnixNano()), postAttrs,
		metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, true,
	)))
	require.NoError(t, err)
	require.Len(t, getSamples, 1)
	require.Len(t, postSamples, 1)

	assert.Equal(t, getSamples[0].EntityID, postSamples[0].EntityID)
	assert.Equal(t, "GET", getSamples[0].Labels["client_http_method"])
	assert.Equal(t, "POST", postSamples[0].Labels["client_http_method"])
	assert.NotEqual(t, getSamples[0].Labels, postSamples[0].Labels)
}

func TestOTLPNormalizer_ReplayedDeltaDeliveryIsIdempotent(t *testing.T) {
	normalizer := newNormalizer()
	now := time.Now()
	attrs := map[string]string{
		"oksg_source":      metrics.SourceTracesServicegraph,
		"client":           "service-a",
		"server":           "service-b",
		"client_namespace": "default",
		"server_namespace": "default",
		"http.method":      "GET",
	}
	env1 := makeEnvelope(t, serviceResource(), makeSumRMIntWithAttrs(
		"traces_service_graph_request_failed_total",
		8,
		uint64(now.UnixNano()),
		attrs,
		metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
		false,
	))
	env2 := makeEnvelope(t, serviceResource(), makeSumRMIntWithAttrs(
		"traces_service_graph_request_failed_total",
		8,
		uint64(now.UnixNano()),
		attrs,
		metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA,
		false,
	))

	first, err := normalizer.Normalize(context.Background(), env1)
	require.NoError(t, err)
	second, err := normalizer.Normalize(context.Background(), env2)
	require.NoError(t, err)

	require.Len(t, first, 1)
	require.Len(t, second, 1)
	assert.Equal(t, 8.0, first[0].Value)
	assert.Equal(t, 8.0, second[0].Value)
	assert.Equal(t, first[0], second[0])
	assert.Equal(t, "delta", second[0].Labels["temporality"])
}

func TestOTLPNormalizer_ServicegraphDoesNotInventDeltaTemporality(t *testing.T) {
	normalizer := newNormalizer()
	now := time.Now()
	attrs := map[string]string{
		"oksg_source":      metrics.SourceTracesServicegraph,
		"client":           "service-a",
		"server":           "service-b",
		"client_namespace": "default",
		"server_namespace": "default",
	}
	env1 := makeEnvelope(t, serviceResource(), makeGaugeRMWithAttrs(
		"traces_service_graph_request_total",
		4,
		uint64(now.UnixNano()),
		attrs,
	))
	env2 := makeEnvelope(t, serviceResource(), makeGaugeRMWithAttrs(
		"traces_service_graph_request_total",
		45,
		uint64(now.Add(15*time.Second).UnixNano()),
		attrs,
	))
	env3 := makeEnvelope(t, serviceResource(), makeGaugeRMWithAttrs(
		"traces_service_graph_request_total",
		5,
		uint64(now.Add(30*time.Second).UnixNano()),
		attrs,
	))

	first, err := normalizer.Normalize(context.Background(), env1)
	require.NoError(t, err)
	second, err := normalizer.Normalize(context.Background(), env2)
	require.NoError(t, err)
	third, err := normalizer.Normalize(context.Background(), env3)
	require.NoError(t, err)

	require.Len(t, first, 1)
	require.Len(t, second, 1)
	require.Len(t, third, 1)
	assert.Equal(t, 4.0, first[0].Value)
	assert.Equal(t, 45.0, second[0].Value)
	assert.Equal(t, 5.0, third[0].Value)
	assert.NotContains(t, third[0].Labels, "temporality")
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
