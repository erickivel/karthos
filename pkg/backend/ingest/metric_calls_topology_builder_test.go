package ingest

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/erickivel/karthos/pkg/backend/persistence"
	"github.com/erickivel/karthos/pkg/core/entities"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/core/schema"
	"github.com/erickivel/karthos/pkg/core/topology"
	"github.com/erickivel/karthos/pkg/sources"
)

func TestMetricCallsTopologyBuilderBuildsCallsEdgeFromIstioMetric(t *testing.T) {
	builder := NewMetricCallsTopologyBuilder("kind-test", NewMetricSourcePolicy(sources.DefaultRegistry().MetricIDs(), nil))
	env := makeCallsMetricEnvelope(t, "istio_requests_total", time.Date(2026, 3, 14, 12, 1, 0, 0, time.UTC), time.Date(2026, 3, 14, 12, 2, 0, 0, time.UTC), map[string]string{
		"client":                          "gateway-go",
		"server":                          "service-a-python",
		"client_namespace":                "karthos-demo",
		"server_namespace":                "karthos-demo",
		"network.protocol.name":           "http",
		"mesh.security.policy":            "mutual_tls",
		"mesh.peer.destination.principal": "spiffe://cluster.local/ns/karthos-demo/sa/service-a",
		"karthos.source":                  "mesh.istio",
	})

	edges, err := builder.Build(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, edges, 1)

	sourceID := entities.ComputeEntityID(entities.KindService, "kind-test", "karthos-demo", "gateway-go")
	targetID := entities.ComputeEntityID(entities.KindService, "kind-test", "karthos-demo", "service-a-python")
	assert.Equal(t, sourceID, edges[0].Source)
	assert.Equal(t, targetID, edges[0].Target)
	assert.Equal(t, topology.RelCalls, edges[0].Relation)
	assert.Equal(t, []string{"mesh.istio"}, edges[0].Provenance)
	assert.Equal(t, "http", edges[0].Metadata["network.protocol.name"])
	assert.Equal(t, "mutual_tls", edges[0].Metadata["mesh.security.policy"])
	assert.Equal(t, "spiffe://cluster.local/ns/karthos-demo/sa/service-a", edges[0].Metadata["mesh.peer.destination.principal"])
}

func TestMetricCallsTopologyBuilderUsesServicegraphSideNamespaces(t *testing.T) {
	builder := NewMetricCallsTopologyBuilder("kind-test", nil)
	at := time.Date(2026, 3, 14, 12, 2, 0, 0, time.UTC)
	env := makeCallsMetricEnvelope(t, "traces_service_graph_request_total", at, at, map[string]string{
		"client": "gateway", "server": "backend",
		"client_k8s.namespace.name": "front", "server_k8s.namespace.name": "back",
		"karthos.source": metrics.SourceTracesServicegraph,
	})
	edges, err := builder.Build(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, edges, 1)
	assert.Equal(t, entities.ComputeEntityID(entities.KindService, "kind-test", "front", "gateway"), edges[0].Source)
	assert.Equal(t, entities.ComputeEntityID(entities.KindService, "kind-test", "back", "backend"), edges[0].Target)
}

func TestMeshAndTraceCallsRemainIndependentEvidence(t *testing.T) {
	ctx := context.Background()
	at := time.Date(2026, 3, 14, 12, 2, 0, 0, time.UTC)
	mesh := makeCallsMetricEnvelope(t, "istio_requests_total", at, at, map[string]string{
		"client": "gateway", "server": "backend", "client_namespace": "default", "server_namespace": "default", "karthos.source": metrics.SourceMeshIstio,
	})
	meshEdges, err := NewMetricCallsTopologyBuilder("kind-test", nil).Build(ctx, mesh)
	require.NoError(t, err)
	require.Len(t, meshEdges, 1)
	traceEnvelope := makeTraceEnvelope(t, "otlp/grpc", map[string]string{"service.name": "gateway", "k8s.namespace.name": "default"}, &tracepb.ResourceSpans{ScopeSpans: []*tracepb.ScopeSpans{{Spans: []*tracepb.Span{makeNamespacedSpan("backend", uint64(at.UnixNano()), uint64(at.Add(time.Second).UnixNano()))}}}})
	traceEdges, err := NewTraceTopologyBuilder(NewOTLPEntityResolver("kind-test")).Build(ctx, traceEnvelope)
	require.NoError(t, err)
	require.Len(t, traceEdges, 1)
	require.Equal(t, meshEdges[0].ID, traceEdges[0].ID)
	store := persistence.NewInMemoryGraphStore()
	require.NoError(t, store.PutObservedEdges(ctx, meshEdges))
	require.NoError(t, store.PutObservedEdges(ctx, traceEdges))
	for _, source := range []string{metrics.SourceMeshIstio, metrics.SourceTracesOTLP} {
		observed, err := store.ObservedEdgesInWindow(ctx, at.Add(-time.Second), at.Add(2*time.Second), source)
		require.NoError(t, err)
		require.Len(t, observed, 1)
		require.Equal(t, []string{source}, observed[0].Provenance)
	}
}

func TestMetricCallsTopologyBuilderUsesDataPointTimestampForCallsEdge(t *testing.T) {
	builder := NewMetricCallsTopologyBuilder("kind-test", NewMetricSourcePolicy(sources.DefaultRegistry().MetricIDs(), nil))
	envelopeTimestamp := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	dataPointTimestamp := envelopeTimestamp.Add(5 * time.Minute)
	env := makeCallsMetricEnvelope(t, "istio_requests_total", envelopeTimestamp, dataPointTimestamp, map[string]string{
		"client":           "gateway-go",
		"server":           "service-a-python",
		"client_namespace": "karthos-demo",
		"server_namespace": "karthos-demo",
		"karthos.source":   "mesh.istio",
	})

	edges, err := builder.Build(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, edges, 1)

	assert.Equal(t, dataPointTimestamp, edges[0].FirstSeen)
	assert.Equal(t, dataPointTimestamp, edges[0].LastSeen)
}

func TestMetricCallsTopologyBuilderDropsTimestamplessMetricEvidence(t *testing.T) {
	builder := NewMetricCallsTopologyBuilder("kind-test", NewMetricSourcePolicy(sources.DefaultRegistry().MetricIDs(), nil))
	env := makeCallsMetricEnvelope(t, "istio_requests_total", time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC), time.Time{}, map[string]string{
		"client":           "gateway-go",
		"server":           "service-a-python",
		"client_namespace": "karthos-demo",
		"server_namespace": "karthos-demo",
		"karthos.source":   "mesh.istio",
	})

	edges, err := builder.Build(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, edges)
}

func TestMetricCallsTopologyBuilderIgnoresMetricsWithoutCallsSemantics(t *testing.T) {
	builder := NewMetricCallsTopologyBuilder("kind-test", NewMetricSourcePolicy(sources.DefaultRegistry().MetricIDs(), nil))
	env := makeCallsMetricEnvelope(t, "k8s.pod.cpu.usage", time.Date(2026, 3, 14, 12, 1, 0, 0, time.UTC), time.Date(2026, 3, 14, 12, 2, 0, 0, time.UTC), map[string]string{
		"client":           "gateway-go",
		"server":           "service-a-python",
		"client_namespace": "karthos-demo",
		"server_namespace": "karthos-demo",
	})

	edges, err := builder.Build(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, edges)
}

func TestMetricCallsTopologyBuilderRejectsInvalidSource(t *testing.T) {
	policy := NewMetricSourcePolicy(sources.DefaultRegistry().MetricIDs(), metrics.Assignment{
		"servicegraph.request.count": metrics.SourceMeshIstio,
	})
	builder := NewMetricCallsTopologyBuilder("kind-test", policy)
	tests := []struct {
		name   string
		source string
	}{
		{name: "missing"},
		{name: "unknown", source: "not-registered"},
		{name: "assignment mismatch", source: metrics.SourceTracesServicegraph},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			attrs := map[string]string{
				"client": "gateway-go", "server": "service-a-python",
				"client_namespace": "karthos-demo", "server_namespace": "karthos-demo",
			}
			if tt.source != "" {
				attrs["karthos.source"] = tt.source
			}
			env := makeCallsMetricEnvelope(t, "istio_requests_total", time.Now(), time.Now(), attrs)
			edges, err := builder.Build(context.Background(), env)
			require.NoError(t, err)
			assert.Empty(t, edges)
		})
	}
}

func TestBufferedTopologyIngestor_KeepsSourceEvidenceSeparate(t *testing.T) {
	store := &recordingGraphStore{}
	now := time.Now().UTC()

	traceEdge := topology.NewEdge("svc-a", "svc-b", topology.RelCalls, now)
	traceEdge.Provenance = []string{metrics.SourceTracesOTLP}
	traceEdge.Metadata = map[string]string{"trace_id": "abc"}
	meshEdge := topology.NewEdge("svc-a", "svc-b", topology.RelCalls, now.Add(time.Second))
	meshEdge.Provenance = []string{"mesh.istio"}
	meshEdge.Metadata = map[string]string{"mesh.security.policy": "mutual_tls"}

	builder := &configuredBuilder{edges: []topology.Edge{traceEdge}}
	buf := NewBufferedTopologyIngestor(map[schema.SignalType]TopologyBuilder{schema.SignalMetric: builder}, store, nil, time.Hour)
	defer buf.Close()

	require.NoError(t, buf.Ingest(context.Background(), newEnvelope(t, schema.SignalMetric, now)))
	builder.edges = []topology.Edge{meshEdge}
	require.NoError(t, buf.Ingest(context.Background(), newEnvelope(t, schema.SignalMetric, now.Add(time.Second))))

	buf.Flush()
	require.Equal(t, 1, store.count())
	edges := store.last().Graph.Edges
	require.Len(t, edges, 2)
	bySource := map[string]topology.Edge{}
	for _, edge := range edges {
		bySource[edge.Provenance[0]] = edge
	}
	assert.Equal(t, "abc", bySource[metrics.SourceTracesOTLP].Metadata["trace_id"])
	assert.Equal(t, now, bySource[metrics.SourceTracesOTLP].LastSeen)
	assert.Equal(t, "mutual_tls", bySource[metrics.SourceMeshIstio].Metadata["mesh.security.policy"])
	assert.Equal(t, now.Add(time.Second), bySource[metrics.SourceMeshIstio].LastSeen)
}

func makeCallsMetricEnvelope(t *testing.T, metricName string, envelopeTimestamp, dataPointTimestamp time.Time, attrs map[string]string) schema.RawEnvelope {
	t.Helper()
	dataPointUnixNano := uint64(0)
	if !dataPointTimestamp.IsZero() {
		dataPointUnixNano = uint64(dataPointTimestamp.UnixNano())
	}
	var kvs []*commonpb.KeyValue
	for k, v := range attrs {
		kvs = append(kvs, &commonpb.KeyValue{
			Key:   k,
			Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}},
		})
	}
	rm := &metricspb.ResourceMetrics{
		ScopeMetrics: []*metricspb.ScopeMetrics{{
			Metrics: []*metricspb.Metric{{
				Name: metricName,
				Data: &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{
					DataPoints: []*metricspb.NumberDataPoint{{
						Attributes:   kvs,
						Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 1},
						TimeUnixNano: dataPointUnixNano,
					}},
				}},
			}},
		}},
	}
	payload, err := protojson.Marshal(rm)
	require.NoError(t, err)
	env, err := schema.NewRawEnvelope("otlp/grpc", schema.SignalMetric, map[string]string{}, json.RawMessage(payload), envelopeTimestamp)
	require.NoError(t, err)
	return env
}
