package ingest

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/erickivel/karthos/pkg/core/entities"
	"github.com/erickivel/karthos/pkg/core/schema"
	"github.com/erickivel/karthos/pkg/core/topology"
)

func makeTraceEnvelope(t *testing.T, source string, resource map[string]string, rs *tracepb.ResourceSpans) schema.RawEnvelope {
	t.Helper()
	payload, err := protojson.Marshal(rs)
	require.NoError(t, err)
	envelope, err := schema.NewRawEnvelope(source, schema.SignalTrace, resource, payload, time.Now().UTC())
	require.NoError(t, err)
	return envelope
}

func makeSpan(peerService string, startNano, endNano uint64) *tracepb.Span {
	span := &tracepb.Span{
		StartTimeUnixNano: startNano,
		EndTimeUnixNano:   endNano,
	}
	if peerService != "" {
		span.Attributes = []*commonpb.KeyValue{
			{
				Key: "peer.service",
				Value: &commonpb.AnyValue{
					Value: &commonpb.AnyValue_StringValue{StringValue: peerService},
				},
			},
		}
	}
	return span
}

func TestTraceTopologyBuilder_ClientSpanWithPeerService_EdgeCreated(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	builder := NewTraceTopologyBuilder(resolver)

	resource := map[string]string{"service.name": "svc-a"}
	rs := &tracepb.ResourceSpans{
		ScopeSpans: []*tracepb.ScopeSpans{
			{
				Spans: []*tracepb.Span{
					makeSpan("svc-b", 1000000, 2000000),
				},
			},
		},
	}

	envelope := makeTraceEnvelope(t, "test-source", resource, rs)
	edges, err := builder.Build(context.Background(), envelope)
	require.NoError(t, err)
	require.Len(t, edges, 1)

	edge := edges[0]
	assert.Equal(t, topology.RelCalls, edge.Relation)

	callerID, _ := resolver.Resolve(context.Background(), map[string]string{"service.name": "svc-a"})
	calleeID, _ := resolver.Resolve(context.Background(), map[string]string{"service.name": "svc-b"})
	assert.Equal(t, callerID, edge.Source)
	assert.Equal(t, calleeID, edge.Target)
}

func TestTraceTopologyBuilder_NoPeerService_NoEdge(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	builder := NewTraceTopologyBuilder(resolver)

	resource := map[string]string{"service.name": "svc-a"}
	rs := &tracepb.ResourceSpans{
		ScopeSpans: []*tracepb.ScopeSpans{
			{
				Spans: []*tracepb.Span{
					makeSpan("", 1000000, 2000000),
				},
			},
		},
	}

	envelope := makeTraceEnvelope(t, "test-source", resource, rs)
	edges, err := builder.Build(context.Background(), envelope)
	require.NoError(t, err)
	assert.Empty(t, edges)
}

func TestTraceTopologyBuilder_Deduplication_LastSeenUpdated(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	builder := NewTraceTopologyBuilder(resolver)

	resource := map[string]string{"service.name": "svc-a"}

	span1End := uint64(2_000_000_000) // 2 seconds in ns
	span2End := uint64(5_000_000_000) // 5 seconds in ns

	rs := &tracepb.ResourceSpans{
		ScopeSpans: []*tracepb.ScopeSpans{
			{
				Spans: []*tracepb.Span{
					makeSpan("svc-b", 1_000_000_000, span1End),
					makeSpan("svc-b", 3_000_000_000, span2End),
				},
			},
		},
	}

	envelope := makeTraceEnvelope(t, "test-source", resource, rs)
	edges, err := builder.Build(context.Background(), envelope)
	require.NoError(t, err)
	require.Len(t, edges, 1)

	expectedLastSeen := time.Unix(0, int64(span2End)).UTC()
	assert.Equal(t, expectedLastSeen, edges[0].LastSeen)
}

func TestTraceTopologyBuilder_MultipleDistinctCallees_TwoEdges(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	builder := NewTraceTopologyBuilder(resolver)

	resource := map[string]string{"service.name": "svc-a"}
	rs := &tracepb.ResourceSpans{
		ScopeSpans: []*tracepb.ScopeSpans{
			{
				Spans: []*tracepb.Span{
					makeSpan("svc-b", 1000000, 2000000),
					makeSpan("svc-c", 3000000, 4000000),
				},
			},
		},
	}

	envelope := makeTraceEnvelope(t, "test-source", resource, rs)
	edges, err := builder.Build(context.Background(), envelope)
	require.NoError(t, err)
	assert.Len(t, edges, 2)

	targets := make(map[string]bool)
	callerID, _ := resolver.Resolve(context.Background(), map[string]string{"service.name": "svc-a"})
	for _, e := range edges {
		assert.Equal(t, callerID, e.Source)
		assert.Equal(t, topology.RelCalls, e.Relation)
		targets[string(e.Target)] = true
	}
	assert.Len(t, targets, 2)
}

func TestTraceTopologyBuilder_MissingServiceName_NilNil(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	builder := NewTraceTopologyBuilder(resolver)

	resource := map[string]string{"k8s.namespace.name": "default"}
	rs := &tracepb.ResourceSpans{
		ScopeSpans: []*tracepb.ScopeSpans{
			{
				Spans: []*tracepb.Span{
					makeSpan("svc-b", 1000000, 2000000),
				},
			},
		},
	}

	envelope := makeTraceEnvelope(t, "test-source", resource, rs)
	edges, err := builder.Build(context.Background(), envelope)
	require.NoError(t, err)
	assert.Nil(t, edges)
}

func TestTraceTopologyBuilder_EnvelopeProvenance(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	builder := NewTraceTopologyBuilder(resolver)

	source := "otel-collector-svc-a"
	resource := map[string]string{"service.name": "svc-a"}
	rs := &tracepb.ResourceSpans{
		ScopeSpans: []*tracepb.ScopeSpans{
			{
				Spans: []*tracepb.Span{
					makeSpan("svc-b", 1000000, 2000000),
				},
			},
		},
	}

	envelope := makeTraceEnvelope(t, source, resource, rs)
	edges, err := builder.Build(context.Background(), envelope)
	require.NoError(t, err)
	require.Len(t, edges, 1)

	assert.Equal(t, []string{source}, edges[0].Provenance)
}

func TestTraceTopologyBuilder_PrefersServiceEntityOverNodeForCaller(t *testing.T) {
	resolver := NewOTLPEntityResolver(testCluster)
	builder := NewTraceTopologyBuilder(resolver)

	resource := map[string]string{
		"service.name":       "svc-a",
		"k8s.node.name":      "node-1",
		"k8s.namespace.name": "default",
	}
	rs := &tracepb.ResourceSpans{
		ScopeSpans: []*tracepb.ScopeSpans{
			{
				Spans: []*tracepb.Span{
					makeSpan("svc-b", 1000000, 2000000),
				},
			},
		},
	}

	envelope := makeTraceEnvelope(t, "test-source", resource, rs)
	edges, err := builder.Build(context.Background(), envelope)
	require.NoError(t, err)
	require.Len(t, edges, 1)

	expectedCaller := entities.ComputeEntityID(entities.KindService, testCluster, "default", "svc-a")
	expectedCallee := entities.ComputeEntityID(entities.KindService, testCluster, "default", "svc-b")

	assert.Equal(t, expectedCaller, edges[0].Source)
	assert.Equal(t, expectedCallee, edges[0].Target)
}
