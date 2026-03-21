package ingest

import (
	"context"
	"fmt"
	"time"

	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/topology"
)

// TraceTopologyBuilder builds topology edges from OTLP trace envelopes by detecting
// cross-service calls via peer.service span attributes.
type TraceTopologyBuilder struct {
	resolver EntityResolver
}

// NewTraceTopologyBuilder creates a new TraceTopologyBuilder backed by the given EntityResolver.
func NewTraceTopologyBuilder(resolver EntityResolver) *TraceTopologyBuilder {
	return &TraceTopologyBuilder{resolver: resolver}
}

// Build deserializes a protojson-encoded ResourceSpans from the envelope payload and
// extracts topology edges for each cross-service call found in the spans.
func (b *TraceTopologyBuilder) Build(ctx context.Context, envelope schema.RawEnvelope) ([]topology.Edge, error) {
	var rs tracepb.ResourceSpans
	if err := protojson.Unmarshal(envelope.Payload, &rs); err != nil {
		return nil, fmt.Errorf("unmarshal ResourceSpans: %w", err)
	}

	callerServiceName, ok := envelope.Resource["service.name"]
	if !ok || callerServiceName == "" {
		return nil, nil
	}

	callerID, err := b.resolver.Resolve(ctx, envelope.Resource)
	if err != nil {
		return nil, err
	}

	seen := make(map[string]topology.Edge)

	for _, ss := range rs.ScopeSpans {
		for _, span := range ss.Spans {
			calleeServiceName := kvString(span.Attributes, "peer.service")
			if calleeServiceName == "" {
				continue
			}

			calleeResource := map[string]string{
				"service.name": calleeServiceName,
			}
			if ns, hasNS := envelope.Resource["k8s.namespace.name"]; hasNS {
				calleeResource["k8s.namespace.name"] = ns
			}

			calleeID, err := b.resolver.Resolve(ctx, calleeResource)
			if err != nil {
				continue
			}

			var firstSeen, lastSeen time.Time
			if span.StartTimeUnixNano != 0 {
				firstSeen = time.Unix(0, int64(span.StartTimeUnixNano)).UTC() //#nosec G115 -- Unix nanosecond timestamps from real systems won't overflow int64 before year 2262
			} else {
				firstSeen = envelope.Timestamp
			}
			if span.EndTimeUnixNano != 0 {
				lastSeen = time.Unix(0, int64(span.EndTimeUnixNano)).UTC() //#nosec G115 -- Unix nanosecond timestamps from real systems won't overflow int64 before year 2262
			} else {
				lastSeen = envelope.Timestamp
			}

			edgeID := topology.ComputeEdgeID(callerID, calleeID, topology.RelCalls)
			if existing, exists := seen[edgeID]; exists {
				if lastSeen.After(existing.LastSeen) {
					existing.LastSeen = lastSeen
					seen[edgeID] = existing
				}
			} else {
				edge := topology.NewEdge(callerID, calleeID, topology.RelCalls, firstSeen)
				edge.LastSeen = lastSeen
				edge.Provenance = []string{envelope.Source}
				edge.Confidence = 1.0
				seen[edgeID] = edge
			}
		}
	}

	if len(seen) == 0 {
		return nil, nil
	}

	edges := make([]topology.Edge, 0, len(seen))
	for _, edge := range seen {
		edges = append(edges, edge)
	}
	return edges, nil
}
