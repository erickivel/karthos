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
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/topology"
)

func TestMeshMetricTopologyBuilderBuildsCallsEdgeFromIstioServicegraphMetric(t *testing.T) {
	builder := NewMeshMetricTopologyBuilder(metrics.NewDialectRegistry(metrics.IstioDialect), "kind-test")
	env := makeMeshMetricEnvelope(t, "istio_requests_total", time.Date(2026, 3, 14, 12, 1, 0, 0, time.UTC), time.Date(2026, 3, 14, 12, 2, 0, 0, time.UTC), map[string]string{
		"client":                          "gateway-go",
		"server":                          "service-a-python",
		"client_namespace":                "oksg-demo",
		"server_namespace":                "oksg-demo",
		"network.protocol.name":           "http",
		"mesh.security.policy":            "mutual_tls",
		"mesh.peer.destination.principal": "spiffe://cluster.local/ns/oksg-demo/sa/service-a",
		"oksg.source":                     "mesh.istio",
	})

	edges, err := builder.Build(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, edges, 1)

	sourceID := entities.ComputeEntityID(entities.KindService, "kind-test", "oksg-demo", "gateway-go")
	targetID := entities.ComputeEntityID(entities.KindService, "kind-test", "oksg-demo", "service-a-python")
	assert.Equal(t, sourceID, edges[0].Source)
	assert.Equal(t, targetID, edges[0].Target)
	assert.Equal(t, topology.RelCalls, edges[0].Relation)
	assert.Equal(t, []string{"mesh.istio"}, edges[0].Provenance)
	assert.Equal(t, "http", edges[0].Metadata["network.protocol.name"])
	assert.Equal(t, "mutual_tls", edges[0].Metadata["mesh.security.policy"])
	assert.Equal(t, "spiffe://cluster.local/ns/oksg-demo/sa/service-a", edges[0].Metadata["mesh.peer.destination.principal"])
}

func TestMeshMetricTopologyBuilderUsesDataPointTimestampForCallsEdge(t *testing.T) {
	builder := NewMeshMetricTopologyBuilder(metrics.NewDialectRegistry(metrics.IstioDialect), "kind-test")
	envelopeTimestamp := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	dataPointTimestamp := envelopeTimestamp.Add(5 * time.Minute)
	env := makeMeshMetricEnvelope(t, "istio_requests_total", envelopeTimestamp, dataPointTimestamp, map[string]string{
		"client":           "gateway-go",
		"server":           "service-a-python",
		"client_namespace": "oksg-demo",
		"server_namespace": "oksg-demo",
		"oksg.source":      "mesh.istio",
	})

	edges, err := builder.Build(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, edges, 1)

	assert.Equal(t, dataPointTimestamp, edges[0].FirstSeen)
	assert.Equal(t, dataPointTimestamp, edges[0].LastSeen)
}

func TestMeshMetricTopologyBuilderIgnoresNonServicegraphMetrics(t *testing.T) {
	builder := NewMeshMetricTopologyBuilder(metrics.NewDialectRegistry(metrics.IstioDialect), "kind-test")
	env := makeMeshMetricEnvelope(t, "k8s.pod.cpu.usage", time.Date(2026, 3, 14, 12, 1, 0, 0, time.UTC), time.Date(2026, 3, 14, 12, 2, 0, 0, time.UTC), map[string]string{
		"client":           "gateway-go",
		"server":           "service-a-python",
		"client_namespace": "oksg-demo",
		"server_namespace": "oksg-demo",
	})

	edges, err := builder.Build(context.Background(), env)
	require.NoError(t, err)
	assert.Empty(t, edges)
}

func TestBufferedTopologyIngestor_MergesProvenanceAndMetadata(t *testing.T) {
	store := &recordingGraphStore{}
	now := time.Now().UTC()

	traceEdge := topology.NewEdge("svc-a", "svc-b", topology.RelCalls, now)
	traceEdge.Provenance = []string{"otlp/grpc"}
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
	edge := store.last().Graph.Edges[0]
	assert.ElementsMatch(t, []string{"otlp/grpc", "mesh.istio"}, edge.Provenance)
	assert.Equal(t, "abc", edge.Metadata["trace_id"])
	assert.Equal(t, "mutual_tls", edge.Metadata["mesh.security.policy"])
	assert.Equal(t, now.Add(time.Second), edge.LastSeen)
}

func makeMeshMetricEnvelope(t *testing.T, metricName string, envelopeTimestamp, dataPointTimestamp time.Time, attrs map[string]string) schema.RawEnvelope {
	t.Helper()
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
						TimeUnixNano: uint64(dataPointTimestamp.UnixNano()),
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
