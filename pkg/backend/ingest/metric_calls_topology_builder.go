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
	"github.com/erickivel/oksg/pkg/sources/contract"
)

// MetricCallsTopologyBuilder derives calls edges from metrics classified by
// their source module. It owns generic provenance validation, edge assembly,
// and deduplication; source modules own metric recognition and metadata.
type MetricCallsTopologyBuilder struct {
	clusterName  string
	sourcePolicy *MetricSourcePolicy
	modules      sources.Registry
}

func NewMetricCallsTopologyBuilder(clusterName string, policy *MetricSourcePolicy) *MetricCallsTopologyBuilder {
	modules := sources.DefaultRegistry()
	if policy == nil {
		policy = NewMetricSourcePolicy(modules.MetricIDs(), nil)
	}
	return &MetricCallsTopologyBuilder{clusterName: clusterName, sourcePolicy: policy, modules: modules}
}

func (b *MetricCallsTopologyBuilder) Build(_ context.Context, envelope schema.RawEnvelope) ([]topology.Edge, error) {
	var rm metricspb.ResourceMetrics
	if err := protojson.Unmarshal(envelope.Payload, &rm); err != nil {
		return nil, fmt.Errorf("unmarshal ResourceMetrics: %w", err)
	}

	seen := make(map[string]topology.Edge)
	for _, sm := range rm.ScopeMetrics {
		for _, metric := range sm.Metrics {
			for _, dp := range metricAttributeSets(metric) {
				if dp.TimeUnixNano == 0 {
					continue
				}
				parsed, validation, ok := b.parseAndValidate(metric.Name, dp.Attributes)
				if !ok {
					continue
				}
				metadata, ok := b.modules.CallsEdgeMetadata(validation.Source, metric.Name, parsed.CanonicalName, dp.Attributes)
				if !ok {
					continue
				}
				edge, ok := b.edgeFromAttributes(time.Unix(0, int64(dp.TimeUnixNano)).UTC(), dp.Attributes, metadata) // #nosec G115 -- Unix nanosecond timestamps from real systems won't overflow int64 before year 2262
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

func (b *MetricCallsTopologyBuilder) parseAndValidate(rawMetricName string, attrs []*commonpb.KeyValue) (contract.ParsedMetric, SourceValidation, bool) {
	parsed, ok := b.modules.ParseMetric(sourceFromAttrs(attrs), rawMetricName, contract.AttributeStrings(attrs))
	if !ok {
		return contract.ParsedMetric{}, SourceValidation{}, false
	}
	if _, recognized := metrics.CatalogByName()[parsed.CanonicalName]; !recognized {
		return contract.ParsedMetric{}, SourceValidation{}, false
	}
	validation := b.sourcePolicy.Validate(parsed.CanonicalName, attrs)
	if !validation.Accepted() {
		return contract.ParsedMetric{}, SourceValidation{}, false
	}
	return parsed, validation, true
}

func (b *MetricCallsTopologyBuilder) edgeFromAttributes(timestamp time.Time, attrs []*commonpb.KeyValue, metadata map[string]string) (topology.Edge, bool) {
	client := kvString(attrs, semconv.AttrClient)
	server := kvString(attrs, semconv.AttrServer)
	clientNamespace := kvString(attrs, semconv.AttrClientNamespace)
	serverNamespace := kvString(attrs, semconv.AttrServerNamespace)
	if client == "" || server == "" || clientNamespace == "" || serverNamespace == "" {
		return topology.Edge{}, false
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
