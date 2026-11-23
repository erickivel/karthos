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
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
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

func TestMetricCallsTopologyBuilderObservesOnlyIncreases(t *testing.T) {
	for _, tt := range []struct {
		source, raw string
		histogram   bool
	}{
		{metrics.SourceMeshIstio, "istio_requests_total", false},
		{metrics.SourceTracesServicegraph, "traces_service_graph_request_total", false},
		{metrics.SourceTracesServicegraph, "traces_service_graph_request_server", true},
	} {
		t.Run(tt.raw, func(t *testing.T) {
			ctx := context.Background()
			at := time.Now().UTC()
			store := persistence.NewInMemoryGraphStore()
			builder := NewMetricCallsTopologyBuilder("kind-test", nil)
			buffer := NewBufferedTopologyIngestor(map[schema.SignalType]TopologyBuilder{schema.SignalMetric: builder}, store, nil, time.Hour)
			defer buffer.Close()

			for i, value := range []uint64{5, 5, 5, 5, 7, 7, 9, 9, 9} {
				ts := at.Add(time.Duration(i) * time.Second)
				env := callsEnvelope(t, tt.raw, ts, tt.source, "", tt.histogram, cumulativeCalls,
					callTestPoint{at: ts, value: value})
				require.NoError(t, buffer.Ingest(ctx, env))
				if i == 3 {
					buffer.Flush()
					rows, err := store.ObservedEdgesInWindow(ctx, at, ts, tt.source)
					require.NoError(t, err)
					require.Empty(t, rows)
				}
			}
			buffer.Flush()
			rows, err := store.ObservedEdgesInWindow(ctx, at, at.Add(8*time.Second), tt.source)
			require.NoError(t, err)
			require.Len(t, rows, 1)
			assert.Equal(t, at.Add(4*time.Second), rows[0].FirstSeen)
			assert.Equal(t, at.Add(6*time.Second), rows[0].LastSeen)
			idle, err := store.ObservedEdgesInWindow(ctx, at.Add(7*time.Second), at.Add(8*time.Second), tt.source)
			require.NoError(t, err)
			require.Empty(t, idle)
		})
	}
}

func TestMetricCallsTopologyBuilderHandlesSeriesLifecycle(t *testing.T) {
	ctx := context.Background()
	at := time.Now().UTC()
	builder := NewMetricCallsTopologyBuilder("kind-test", nil)
	builder.series.startedAt = at
	now := at
	builder.series.now = func() time.Time { return now }
	for _, tt := range []struct {
		name       string
		producer   string
		end, start int
		count      uint64
		evidence   bool
	}{
		{name: "old first value", producer: "a", end: 10, start: -1, count: 5},
		{name: "increment", producer: "a", end: 11, start: -1, count: 6, evidence: true},
		{name: "duplicate", producer: "a", end: 11, start: -1, count: 6},
		{name: "late lower sample", producer: "a", end: 9, start: -1, count: 2},
		{name: "same value after late sample", producer: "a", end: 12, start: -1, count: 6},
		{name: "second producer has its own baseline", producer: "b", end: 12, start: -1, count: 2},
		{name: "positive reset by value", producer: "a", end: 13, start: -1, count: 3, evidence: true},
		{name: "reset by explicit new start", producer: "a", end: 15, start: 14, count: 3, evidence: true},
		{name: "unknown start is a baseline", producer: "a", end: 16, start: 16, count: 2},
		{name: "explicit new first series", producer: "c", end: 16, start: 15, count: 1, evidence: true},
		{name: "second producer increments", producer: "b", end: 16, start: -1, count: 3, evidence: true},
		{name: "reset to zero", producer: "a", end: 18, start: 17},
		{name: "first call after zero", producer: "a", end: 19, start: 17, count: 1, evidence: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			now = at.Add(time.Duration(tt.end) * time.Second)
			env := callsEnvelope(t, "istio_requests_total", now, metrics.SourceMeshIstio, tt.producer, false, cumulativeCalls,
				callTestPoint{at: now, start: at.Add(time.Duration(tt.start) * time.Second), value: tt.count})
			edges, err := builder.Build(ctx, env)
			require.NoError(t, err)
			if tt.evidence {
				require.Len(t, edges, 1)
				require.Equal(t, now, edges[0].LastSeen)
			} else {
				require.Empty(t, edges)
			}
		})
	}

	now = at.Add(topology.HistoryRetention + time.Minute)
	env := callsEnvelope(t, "istio_requests_total", now, metrics.SourceMeshIstio, "a", false, cumulativeCalls,
		callTestPoint{at: now, value: 20})
	edges, err := builder.Build(ctx, env)
	require.NoError(t, err)
	require.Empty(t, edges, "an expired series needs a new baseline")
	require.Len(t, builder.series.entries, 1, "idle producers were removed")
}

func TestMetricCallsTopologyBuilderSortsPointsAndPreservesExactHistogramCounts(t *testing.T) {
	at := time.Now().UTC()
	builder := NewMetricCallsTopologyBuilder("kind-test", nil)
	base := uint64(1 << 53)
	env := callsEnvelope(t, "traces_service_graph_request_server", at, metrics.SourceTracesServicegraph, "", true, cumulativeCalls,
		callTestPoint{at: at.Add(3 * time.Second), value: base + 2},
		callTestPoint{at: at.Add(time.Second), value: base},
		callTestPoint{at: at.Add(2 * time.Second), value: base + 1},
	)
	edges, err := builder.Build(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, edges, 1)
	require.Equal(t, at.Add(2*time.Second), edges[0].FirstSeen)
	require.Equal(t, at.Add(3*time.Second), edges[0].LastSeen)
}

func TestMetricCallsTopologyBuilderFiltersUnsupportedEvidence(t *testing.T) {
	for _, tt := range []struct {
		name, raw string
		flags     uint32
		mutate    func(*metricspb.Metric)
	}{
		{name: "zero delta", raw: "istio_requests_total", mutate: func(m *metricspb.Metric) {
			m.GetSum().DataPoints[0].Value = &metricspb.NumberDataPoint_AsInt{}
		}},
		{name: "gauge", raw: "istio_requests_total", mutate: func(m *metricspb.Metric) {
			m.Data = &metricspb.Metric_Gauge{Gauge: &metricspb.Gauge{DataPoints: m.GetSum().DataPoints}}
		}},
		{name: "nonmonotonic sum", raw: "istio_requests_total", mutate: func(m *metricspb.Metric) {
			m.GetSum().IsMonotonic = false
		}},
		{name: "missing value flag", raw: "istio_requests_total", flags: uint32(metricspb.DataPointFlags_DATA_POINT_FLAGS_NO_RECORDED_VALUE_MASK)},
		{name: "tcp byte counter", raw: "istio_tcp_sent_bytes_total"},
		{name: "tcp close counter", raw: "istio_tcp_connections_closed_total"},
		{name: "grpc messages", raw: "istio_request_messages_total"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			at := time.Now().UTC()
			env := callsEnvelope(t, tt.raw, at, metrics.SourceMeshIstio, "", false, deltaCalls,
				callTestPoint{at: at, value: 1, flags: tt.flags})
			if tt.mutate != nil {
				var rm metricspb.ResourceMetrics
				require.NoError(t, protojson.Unmarshal(env.Payload, &rm))
				tt.mutate(rm.ScopeMetrics[0].Metrics[0])
				env = marshalCallsEnvelope(t, &rm, at)
			}
			builder := NewMetricCallsTopologyBuilder("kind-test", nil)
			edges, err := builder.Build(context.Background(), env)
			require.NoError(t, err)
			require.Empty(t, edges)
		})
	}
}

func TestMetricCallsTopologyBuilderDeduplicatesConcurrentDelta(t *testing.T) {
	at := time.Now().UTC()
	env := callsEnvelope(t, "istio_tcp_connections_opened_total", at, metrics.SourceMeshIstio, "", false, deltaCalls,
		callTestPoint{at: at, value: 1})
	builder := NewMetricCallsTopologyBuilder("kind-test", nil)
	results := make(chan struct{}, 8)
	t.Run("concurrent delivery", func(t *testing.T) {
		for range 8 {
			t.Run("export", func(t *testing.T) {
				t.Parallel()
				edges, err := builder.Build(context.Background(), env)
				require.NoError(t, err)
				if len(edges) != 0 {
					results <- struct{}{}
				}
			})
		}
	})
	require.Len(t, results, 1)
}

const (
	cumulativeCalls = metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE
	deltaCalls      = metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA
)

type callTestPoint struct {
	at, start time.Time
	value     uint64
	flags     uint32
}

func callsEnvelope(t *testing.T, raw string, received time.Time, source, producer string, histogram bool, temporality metricspb.AggregationTemporality, points ...callTestPoint) schema.RawEnvelope {
	t.Helper()
	attrs := callAttributes(map[string]string{
		"client": "client", "server": "server", "client_namespace": "default",
		"server_namespace": "default", "karthos.source": source,
	})
	metric := &metricspb.Metric{Name: raw}
	if histogram {
		data := &metricspb.Histogram{AggregationTemporality: temporality}
		for _, point := range points {
			data.DataPoints = append(data.DataPoints, &metricspb.HistogramDataPoint{
				Attributes: attrs, Count: point.value, StartTimeUnixNano: callTestTimestamp(point.start),
				TimeUnixNano: callTestTimestamp(point.at), Flags: point.flags,
			})
		}
		metric.Data = &metricspb.Metric_Histogram{Histogram: data}
	} else {
		data := &metricspb.Sum{AggregationTemporality: temporality, IsMonotonic: true}
		for _, point := range points {
			data.DataPoints = append(data.DataPoints, &metricspb.NumberDataPoint{
				Attributes: attrs, Value: &metricspb.NumberDataPoint_AsInt{AsInt: int64(point.value)},
				StartTimeUnixNano: callTestTimestamp(point.start), TimeUnixNano: callTestTimestamp(point.at), Flags: point.flags,
			})
		}
		metric.Data = &metricspb.Metric_Sum{Sum: data}
	}
	rm := &metricspb.ResourceMetrics{
		Resource:     &resourcepb.Resource{Attributes: callAttributes(map[string]string{"service.instance.id": producer})},
		ScopeMetrics: []*metricspb.ScopeMetrics{{Metrics: []*metricspb.Metric{metric}}},
	}
	return marshalCallsEnvelope(t, rm, received)
}

func callTestTimestamp(at time.Time) uint64 {
	if at.IsZero() {
		return 0
	}
	return uint64(at.UnixNano())
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
	rm := &metricspb.ResourceMetrics{
		ScopeMetrics: []*metricspb.ScopeMetrics{{
			Metrics: []*metricspb.Metric{{
				Name: metricName,
				Data: &metricspb.Metric_Sum{Sum: &metricspb.Sum{
					AggregationTemporality: deltaCalls,
					IsMonotonic:            true,
					DataPoints: []*metricspb.NumberDataPoint{{
						Attributes:   callAttributes(attrs),
						Value:        &metricspb.NumberDataPoint_AsDouble{AsDouble: 1},
						TimeUnixNano: callTestTimestamp(dataPointTimestamp),
					}},
				}},
			}},
		}},
	}
	return marshalCallsEnvelope(t, rm, envelopeTimestamp)
}

func callAttributes(attrs map[string]string) []*commonpb.KeyValue {
	kvs := make([]*commonpb.KeyValue, 0, len(attrs))
	for k, v := range attrs {
		kvs = append(kvs, &commonpb.KeyValue{
			Key: k, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: v}},
		})
	}
	return kvs
}

func marshalCallsEnvelope(t *testing.T, rm *metricspb.ResourceMetrics, envelopeTimestamp time.Time) schema.RawEnvelope {
	t.Helper()
	payload, err := protojson.Marshal(rm)
	require.NoError(t, err)
	env, err := schema.NewRawEnvelope("otlp/grpc", schema.SignalMetric, map[string]string{}, json.RawMessage(payload), envelopeTimestamp)
	require.NoError(t, err)
	return env
}
