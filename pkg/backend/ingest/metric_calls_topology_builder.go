package ingest

import (
	"context"
	"fmt"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/semconv"
	"github.com/erickivel/oksg/pkg/core/topology"
	"github.com/erickivel/oksg/pkg/sources"
)

// MetricCallsTopologyBuilder derives calls edges from metrics classified by
// their source module. It owns generic provenance validation, edge assembly,
// and deduplication; source modules own metric recognition and metadata.
type MetricCallsTopologyBuilder struct {
	dialect      *metrics.DialectRegistry
	clusterName  string
	sourcePolicy *MetricSourcePolicy
	modules      sources.Registry
}

func NewMetricCallsTopologyBuilder(dialect *metrics.DialectRegistry, clusterName string, policy *MetricSourcePolicy) *MetricCallsTopologyBuilder {
	modules := sources.DefaultRegistry()
	if policy == nil {
		policy = NewMetricSourcePolicy(modules.MetricIDs(), nil)
	}
	return &MetricCallsTopologyBuilder{dialect: dialect, clusterName: clusterName, sourcePolicy: policy, modules: modules}
}

func (b *MetricCallsTopologyBuilder) Build(_ context.Context, envelope schema.RawEnvelope) ([]topology.Edge, error) {
	var rm metricspb.ResourceMetrics
	if err := protojson.Unmarshal(envelope.Payload, &rm); err != nil {
		return nil, fmt.Errorf("unmarshal ResourceMetrics: %w", err)
	}

	seen := make(map[string]topology.Edge)
	for _, sm := range rm.ScopeMetrics {
		for _, metric := range sm.Metrics {
			translated := b.dialect.Translate(metric.Name)
			if _, recognized := metrics.OTLPSemconvMetrics[translated]; !recognized {
				continue
			}
			for _, dp := range metricAttributeSets(metric) {
				metadata, ok := b.callsEdgeMetadata(metric.Name, translated, dp.Attributes)
				if !ok {
					continue
				}
				edge, ok := b.edgeFromAttributes(metricTimestamp(envelope.Timestamp, dp.TimeUnixNano), dp.Attributes, metadata)
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

func (b *MetricCallsTopologyBuilder) BuildNodes(_ context.Context, envelope schema.RawEnvelope) []topology.Node {
	var rm metricspb.ResourceMetrics
	if err := protojson.Unmarshal(envelope.Payload, &rm); err != nil {
		return nil
	}

	seen := make(map[entities.EntityID]topology.Node)
	for _, sm := range rm.ScopeMetrics {
		for _, metric := range sm.Metrics {
			translated := b.dialect.Translate(metric.Name)
			if _, recognized := metrics.OTLPSemconvMetrics[translated]; !recognized {
				continue
			}
			for _, dp := range metricAttributeSets(metric) {
				if _, ok := b.callsEdgeMetadata(metric.Name, translated, dp.Attributes); !ok {
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

func (b *MetricCallsTopologyBuilder) callsEdgeMetadata(rawMetricName, canonicalMetricName string, attrs []*commonpb.KeyValue) (map[string]string, bool) {
	validation := b.sourcePolicy.Validate(canonicalMetricName, attrs)
	if !validation.Accepted() {
		return nil, false
	}
	return b.modules.CallsEdgeMetadata(validation.Source, rawMetricName, canonicalMetricName, attrs)
}

func (b *MetricCallsTopologyBuilder) edgeFromAttributes(timestamp time.Time, attrs []*commonpb.KeyValue, metadata map[string]string) (topology.Edge, bool) {
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
	edge.Metadata = metadata
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
