package ingest

import (
	"context"
	"fmt"
	"strings"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/semconv"
	"github.com/erickivel/oksg/pkg/core/topology"
)

func isServiceGraphEdgeMetric(metricName, translatedName string) bool {
	return strings.HasPrefix(metricName, "traces.service.graph.") ||
		strings.HasPrefix(metricName, "traces_service_graph_") ||
		strings.HasPrefix(metricName, "istio_") ||
		strings.HasPrefix(translatedName, "servicegraph.")
}

// MeshMetricTopologyBuilder derives calls edges from mesh metrics that have
// already been rewritten to the servicegraph edge attribute shape.
type MeshMetricTopologyBuilder struct {
	dialect      *metrics.DialectRegistry
	clusterName  string
	sourcePolicy *MetricSourcePolicy
}

func NewMeshMetricTopologyBuilder(dialect *metrics.DialectRegistry, clusterName string, policy *MetricSourcePolicy) *MeshMetricTopologyBuilder {
	if policy == nil {
		policy = NewMetricSourcePolicy(metrics.DefaultLogicalSources(), nil)
	}
	return &MeshMetricTopologyBuilder{dialect: dialect, clusterName: clusterName, sourcePolicy: policy}
}

func (b *MeshMetricTopologyBuilder) Build(_ context.Context, envelope schema.RawEnvelope) ([]topology.Edge, error) {
	var rm metricspb.ResourceMetrics
	if err := protojson.Unmarshal(envelope.Payload, &rm); err != nil {
		return nil, fmt.Errorf("unmarshal ResourceMetrics: %w", err)
	}

	seen := make(map[string]topology.Edge)
	for _, sm := range rm.ScopeMetrics {
		for _, metric := range sm.Metrics {
			translated := b.dialect.Translate(metric.Name)
			if !isServiceGraphEdgeMetric(metric.Name, translated) {
				continue
			}
			if _, recognized := metrics.OTLPSemconvMetrics[translated]; !recognized {
				continue
			}
			for _, dp := range metricAttributeSets(metric) {
				if !b.sourcePolicy.Validate(translated, dp.Attributes).Accepted() {
					continue
				}
				edge, ok := b.edgeFromAttributes(metricTimestamp(envelope.Timestamp, dp.TimeUnixNano), dp.Attributes)
				if !ok {
					continue
				}
				if existing, exists := seen[edge.ID]; exists {
					existing.Metadata = mergeStringMaps(existing.Metadata, edge.Metadata)
					existing.Provenance = mergeStringSlices(existing.Provenance, edge.Provenance)
					if edge.LastSeen.After(existing.LastSeen) {
						existing.LastSeen = edge.LastSeen
					}
					seen[edge.ID] = existing
				} else {
					seen[edge.ID] = edge
				}
			}
		}
	}

	edges := make([]topology.Edge, 0, len(seen))
	for _, edge := range seen {
		edges = append(edges, edge)
	}
	return edges, nil
}

func (b *MeshMetricTopologyBuilder) BuildNodes(_ context.Context, envelope schema.RawEnvelope) []topology.Node {
	var rm metricspb.ResourceMetrics
	if err := protojson.Unmarshal(envelope.Payload, &rm); err != nil {
		return nil
	}

	seen := make(map[entities.EntityID]topology.Node)
	for _, sm := range rm.ScopeMetrics {
		for _, metric := range sm.Metrics {
			translated := b.dialect.Translate(metric.Name)
			if !isServiceGraphEdgeMetric(metric.Name, translated) {
				continue
			}
			if _, recognized := metrics.OTLPSemconvMetrics[translated]; !recognized {
				continue
			}
			for _, dp := range metricAttributeSets(metric) {
				if !b.sourcePolicy.Validate(translated, dp.Attributes).Accepted() {
					continue
				}
				client := kvString(dp.Attributes, semconv.AttrClient)
				server := kvString(dp.Attributes, semconv.AttrServer)
				clientNamespace := kvString(dp.Attributes, semconv.AttrClientNamespace)
				serverNamespace := kvString(dp.Attributes, semconv.AttrServerNamespace)
				if client != "" && clientNamespace != "" {
					id := entities.ComputeEntityID(entities.KindService, b.clusterName, clientNamespace, client)
					seen[id] = topology.Node{ID: id, Kind: entities.KindService, Labels: map[string]string{"name": client, "namespace": clientNamespace}}
				}
				if server != "" && serverNamespace != "" {
					id := entities.ComputeEntityID(entities.KindService, b.clusterName, serverNamespace, server)
					seen[id] = topology.Node{ID: id, Kind: entities.KindService, Labels: map[string]string{"name": server, "namespace": serverNamespace}}
				}
			}
		}
	}
	nodes := make([]topology.Node, 0, len(seen))
	for _, node := range seen {
		nodes = append(nodes, node)
	}
	return nodes
}

func (b *MeshMetricTopologyBuilder) edgeFromAttributes(timestamp time.Time, attrs []*commonpb.KeyValue) (topology.Edge, bool) {
	client := kvString(attrs, semconv.AttrClient)
	server := kvString(attrs, semconv.AttrServer)
	clientNamespace := kvString(attrs, semconv.AttrClientNamespace)
	serverNamespace := kvString(attrs, semconv.AttrServerNamespace)
	if client == "" || server == "" || clientNamespace == "" || serverNamespace == "" {
		return topology.Edge{}, false
	}

	if timestamp.IsZero() {
		timestamp = time.Now().UTC()
	}
	sourceID := entities.ComputeEntityID(entities.KindService, b.clusterName, clientNamespace, client)
	targetID := entities.ComputeEntityID(entities.KindService, b.clusterName, serverNamespace, server)
	edge := topology.NewEdge(sourceID, targetID, topology.RelCalls, timestamp)
	edge.LastSeen = timestamp
	edge.Confidence = 1.0
	edge.Provenance = []string{sourceFromAttrs(attrs)}
	edge.Metadata = meshMetadata(attrs)
	return edge, true
}

type metricAttributeSet struct {
	Attributes   []*commonpb.KeyValue
	TimeUnixNano uint64
}

func metricAttributeSets(metric *metricspb.Metric) []metricAttributeSet {
	var sets []metricAttributeSet
	if h := metric.GetHistogram(); h != nil {
		for _, dp := range h.DataPoints {
			sets = append(sets, metricAttributeSet{
				Attributes:   dp.Attributes,
				TimeUnixNano: dp.TimeUnixNano,
			})
		}
		return sets
	}
	for _, dp := range collectScalarDataPoints(metric) {
		sets = append(sets, metricAttributeSet{
			Attributes:   dp.Attributes,
			TimeUnixNano: dp.TimeUnixNano,
		})
	}
	return sets
}

func metricTimestamp(fallback time.Time, timeUnixNano uint64) time.Time {
	if timeUnixNano == 0 {
		return fallback
	}
	return time.Unix(0, int64(timeUnixNano)).UTC() //#nosec G115 -- Unix nanosecond timestamps from real systems won't overflow int64 before year 2262
}

func sourceFromAttrs(attrs []*commonpb.KeyValue) string {
	return firstKVString(attrs, semconv.AttrOKSGSource, semconv.AttrOKSGSourceProm)
}

func meshMetadata(attrs []*commonpb.KeyValue) map[string]string {
	metadata := make(map[string]string)
	for _, key := range []string{
		semconv.AttrNetworkProtocolName,
		semconv.AttrHTTPStatusCode,
		semconv.AttrGRPCStatusCode,
		semconv.AttrMeshPolicy,
		semconv.AttrMeshSourcePrincipal,
		semconv.AttrMeshDestinationPrincipal,
		semconv.AttrMeshResponseFlags,
		semconv.AttrMeshID,
	} {
		if value := meshAttrValue(attrs, key); value != "" {
			metadata[key] = value
		}
	}
	return metadata
}

func mergeStringMaps(left, right map[string]string) map[string]string {
	if len(left) == 0 && len(right) == 0 {
		return nil
	}
	merged := make(map[string]string, len(left)+len(right))
	for key, value := range left {
		merged[key] = value
	}
	for key, value := range right {
		if value != "" {
			merged[key] = value
		}
	}
	return merged
}

func mergeStringSlices(left, right []string) []string {
	seen := make(map[string]struct{}, len(left)+len(right))
	var merged []string
	for _, value := range append(left, right...) {
		if value == "" {
			continue
		}
		if _, ok := seen[value]; ok {
			continue
		}
		seen[value] = struct{}{}
		merged = append(merged, value)
	}
	return merged
}
