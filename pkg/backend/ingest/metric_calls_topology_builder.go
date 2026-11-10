package ingest

import (
	"context"
	"fmt"
	"slices"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/erickivel/karthos/pkg/core/entities"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/core/schema"
	"github.com/erickivel/karthos/pkg/core/semconv"
	"github.com/erickivel/karthos/pkg/core/topology"
	"github.com/erickivel/karthos/pkg/sources"
	"github.com/erickivel/karthos/pkg/sources/contract"
)

// MetricCallsTopologyBuilder derives calls edges from metrics classified by
// their source module. It owns generic provenance validation, edge assembly,
// and deduplication; source modules own metric recognition and metadata.
type MetricCallsTopologyBuilder struct {
	clusterName  string
	sourcePolicy *MetricSourcePolicy
	modules      sources.Registry
	catalog      map[string]metrics.CanonicalMetric
	series       callSeriesTracker
}

func NewMetricCallsTopologyBuilder(clusterName string, policy *MetricSourcePolicy) *MetricCallsTopologyBuilder {
	modules := sources.DefaultRegistry()
	if policy == nil {
		policy = NewMetricSourcePolicy(modules.MetricIDs(), nil)
	}
	return &MetricCallsTopologyBuilder{
		clusterName: clusterName, sourcePolicy: policy, modules: modules, catalog: metrics.CatalogByName(),
		series: callSeriesTracker{startedAt: time.Now(), now: time.Now, entries: make(map[callSeriesKey]callSeriesState)},
	}
}

func (b *MetricCallsTopologyBuilder) Build(_ context.Context, envelope schema.RawEnvelope) ([]topology.Edge, error) {
	var rm metricspb.ResourceMetrics
	if err := protojson.Unmarshal(envelope.Payload, &rm); err != nil {
		return nil, fmt.Errorf("unmarshal ResourceMetrics: %w", err)
	}

	resourceKey := callResourceKey(rm.GetResource().GetAttributes(), envelope.Resource)
	var candidates []callObservation
	for _, sm := range rm.ScopeMetrics {
		for _, metric := range sm.Metrics {
			for _, point := range callMetricPoints(metric) {
				parsed, validation, ok := b.parseAndValidate(metric.Name, point.attrs)
				if !ok {
					continue
				}
				metadata, ok := b.modules.CallsEdgeMetadata(validation.Source, metric.Name, parsed.CanonicalName, point.attrs)
				if !ok {
					continue
				}
				clientNamespace, serverNamespace := b.modules.CallsEdgeNamespaces(validation.Source, point.attrs)
				at := time.Unix(0, int64(point.at)).UTC() // #nosec G115 -- callMetricPoints rejects timestamps above MaxInt64
				edge, ok := b.edgeFromAttributes(at, point.attrs, metadata, clientNamespace, serverNamespace)
				if !ok {
					continue
				}
				candidates = append(candidates, callObservation{
					key:   callSeriesKey{metric.Name, validation.Source, resourceKey, callAttributeKey(point.attrs)},
					point: point, edge: edge,
				})
			}
		}
	}

	slices.SortStableFunc(candidates, compareCallObservationTime)
	type observedEdgeKey struct{ id, source string }
	seen := make(map[observedEdgeKey]topology.Edge)
	for _, observation := range b.series.increases(candidates) {
		edge := observation.edge
		key := observedEdgeKey{edge.ID, observation.key.source}
		if existing, ok := seen[key]; ok {
			edge.FirstSeen = existing.FirstSeen
			edge.Metadata = mergeStringMaps(existing.Metadata, edge.Metadata)
		}
		seen[key] = edge
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
	if _, recognized := b.catalog[parsed.CanonicalName]; !recognized {
		return contract.ParsedMetric{}, SourceValidation{}, false
	}
	validation := b.sourcePolicy.Validate(parsed.CanonicalName, attrs)
	if !validation.Accepted() {
		return contract.ParsedMetric{}, SourceValidation{}, false
	}
	return parsed, validation, true
}

func (b *MetricCallsTopologyBuilder) edgeFromAttributes(timestamp time.Time, attrs []*commonpb.KeyValue, metadata map[string]string, sourceClientNamespace, sourceServerNamespace string) (topology.Edge, bool) {
	client := kvString(attrs, semconv.AttrClient)
	server := kvString(attrs, semconv.AttrServer)
	clientNamespace := kvString(attrs, semconv.AttrClientNamespace)
	serverNamespace := kvString(attrs, semconv.AttrServerNamespace)
	if clientNamespace == "" {
		clientNamespace = sourceClientNamespace
	}
	if serverNamespace == "" {
		serverNamespace = sourceServerNamespace
	}
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
	return firstKVString(attrs, semconv.AttrKarthosSource, semconv.AttrKarthosSourceProm)
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
