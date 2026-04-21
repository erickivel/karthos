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
)

// OTLPNormalizer normalizes OTLP metric envelopes into semconv MetricSamples.
type OTLPNormalizer struct {
	resolver EntityResolver
	dialect  *metrics.DialectRegistry
}

// NewOTLPNormalizer creates a new OTLPNormalizer backed by the given EntityResolver.
func NewOTLPNormalizer(resolver EntityResolver, dialect *metrics.DialectRegistry) *OTLPNormalizer {
	if resolver == nil {
		panic("ingest.NewOTLPNormalizer: resolver is required")
	}
	if dialect == nil {
		panic("ingest.NewOTLPNormalizer: dialect registry is required")
	}
	return &OTLPNormalizer{resolver: resolver, dialect: dialect}
}

// Normalize deserializes a protojson-encoded ResourceMetrics from the envelope payload,
// resolves the entity, and maps each recognized metric to a canonical MetricSample.
//
// For histogram-typed metrics the raw cumulative `sum` and `count` are emitted as two
// separate samples (with suffixes `_sum` and `_count`), so that query-time consumers
// can derive a rate-weighted mean via `rate(<metric>_sum[1m]) / rate(<metric>_count[1m])`.
// Emitting a single precomputed mean would collapse a cumulative histogram into a
// lifetime average, which is misleading.
func (n *OTLPNormalizer) Normalize(ctx context.Context, envelope schema.RawEnvelope) ([]metrics.MetricSample, error) {
	var rm metricspb.ResourceMetrics
	if err := protojson.Unmarshal(envelope.Payload, &rm); err != nil {
		return nil, fmt.Errorf("unmarshal ResourceMetrics: %w", err)
	}

	var samples []metrics.MetricSample

	for _, sm := range rm.ScopeMetrics {
		for _, metric := range sm.Metrics {
			translatedName := n.dialect.Translate(metric.Name)
			mapping, ok := metrics.OTLPSemconvMetrics[translatedName]
			if !ok {
				continue
			}

			// Histograms are decomposed into _sum / _count pairs.
			if h := metric.GetHistogram(); h != nil {
				for _, dp := range h.DataPoints {
					if dp.Count == 0 || dp.Sum == nil {
						continue
					}
					metricName := resolveDirectionalName(translatedName, dp.Attributes)
					if metricName == "" {
						continue
					}

					entityID, skip, err := n.resolveMetricEntity(ctx, envelope.Resource, metric.Name, dp.Attributes)
					if err != nil {
						return nil, fmt.Errorf("resolve entity: %w", err)
					}
					if skip {
						continue
					}

					ts := time.Unix(0, int64(dp.TimeUnixNano)).UTC() //#nosec G115 -- Unix nanosecond timestamps from real systems won't overflow int64 before year 2262
					labels := baseLabels(envelope.Source)
					applyServiceGraphEdgeLabels(ctx, n.resolver, envelope.Resource, metric.Name, dp.Attributes, labels)

					samples = append(samples,
						metrics.MetricSample{
							MetricName: metricName + "_sum",
							EntityID:   entityID,
							Value:      *dp.Sum * mapping.ConversionFactor,
							Timestamp:  ts,
							Labels:     labels,
							Unit:       mapping.Unit,
						},
						metrics.MetricSample{
							MetricName: metricName + "_count",
							EntityID:   entityID,
							Value:      float64(dp.Count),
							Timestamp:  ts,
							Labels:     cloneLabels(labels),
							Unit:       mapping.Unit,
						},
					)
				}
				continue
			}

			dps := collectScalarDataPoints(metric)
			for _, dp := range dps {
				ts := time.Unix(0, int64(dp.TimeUnixNano)).UTC() //#nosec G115 -- Unix nanosecond timestamps from real systems won't overflow int64 before year 2262
				value := dp.Value * mapping.ConversionFactor

				metricName := resolveDirectionalName(translatedName, dp.Attributes)
				if metricName == "" {
					continue
				}

				entityID, skip, err := n.resolveMetricEntity(ctx, envelope.Resource, metric.Name, dp.Attributes)
				if err != nil {
					return nil, fmt.Errorf("resolve entity: %w", err)
				}
				if skip {
					continue
				}

				labels := baseLabels(envelope.Source)
				applyServiceGraphEdgeLabels(ctx, n.resolver, envelope.Resource, metric.Name, dp.Attributes, labels)

				samples = append(samples, metrics.MetricSample{
					MetricName: metricName,
					EntityID:   entityID,
					Value:      value,
					Timestamp:  ts,
					Labels:     labels,
					Unit:       mapping.Unit,
				})
			}
		}
	}

	return samples, nil
}

func baseLabels(source string) map[string]string {
	return map[string]string{semconv.LabelSource: source}
}

func cloneLabels(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

// applyServiceGraphEdgeLabels annotates labels with servicegraph client/server identifiers.
// edge_client_id / edge_server_id are only set when the referenced service resolves to a
// canonical Service entity; otherwise only the raw edge_client / edge_server names are
// preserved as context.
func applyServiceGraphEdgeLabels(
	ctx context.Context,
	resolver EntityResolver,
	resource map[string]string,
	metricName string,
	attrs []*commonpb.KeyValue,
	labels map[string]string,
) {
	if !strings.HasPrefix(metricName, "traces.service.graph.") {
		return
	}
	if c := kvString(attrs, semconv.AttrClient); c != "" {
		labels[semconv.LabelEdgeClient] = c
		if clientID, ok := resolveServiceMetricEntityID(ctx, resolver, resource, c, kvString(attrs, semconv.AttrClientNamespace)); ok {
			labels[semconv.LabelEdgeClientID] = string(clientID)
		}
	}
	if s := kvString(attrs, semconv.AttrServer); s != "" {
		labels[semconv.LabelEdgeServer] = s
		if serverID, ok := resolveServiceMetricEntityID(ctx, resolver, resource, s, kvString(attrs, semconv.AttrServerNamespace)); ok {
			labels[semconv.LabelEdgeServerID] = string(serverID)
		}
	}
}

// resolveMetricEntity resolves the entity a metric datapoint belongs to.
//
// Returns (id, skip, err):
//   - skip=true indicates the datapoint must be dropped (e.g. servicegraph client
//     attribute can't be mapped to a canonical Service). Callers must not emit.
//   - skip=false + id populated is the normal success path.
//   - err != nil is a hard ingest failure.
//
// Connector metrics (servicegraph, spanmetrics) are conceptually service-scoped.
// If a canonical Service entity can't be derived, we deliberately do NOT fall
// back to the emitting resource (which is typically the collector's own pod or
// node) — doing so misattributes service traffic to the collector.
func (n *OTLPNormalizer) resolveMetricEntity(
	ctx context.Context,
	resource map[string]string,
	metricName string,
	attrs []*commonpb.KeyValue,
) (entities.EntityID, bool, error) {
	if strings.HasPrefix(metricName, "traces.service.graph.") {
		client := kvString(attrs, semconv.AttrClient)
		if client == "" {
			return "", true, nil
		}
		if clientID, ok := resolveServiceMetricEntityID(ctx, n.resolver, resource, client, kvString(attrs, semconv.AttrClientNamespace)); ok {
			return clientID, false, nil
		}
		return "", true, nil
	}

	if strings.HasPrefix(metricName, "traces.span.metrics.") {
		serviceName := resource[semconv.ResourceServiceName]
		if serviceName == "" {
			return "", true, nil
		}
		if serviceID, ok := resolveServiceMetricEntityID(ctx, n.resolver, resource, serviceName, resource[semconv.ResourceK8sNamespaceName]); ok {
			return serviceID, false, nil
		}
		return "", true, nil
	}

	id, err := n.resolver.Resolve(ctx, resource)
	if err != nil {
		return "", false, err
	}
	return id, false, nil
}

func resolveServiceMetricEntityID(
	ctx context.Context,
	resolver EntityResolver,
	resource map[string]string,
	serviceName, namespaceHint string,
) (entities.EntityID, bool) {
	if serviceName == "" {
		return "", false
	}

	if otlpResolver, ok := resolver.(*OTLPEntityResolver); ok {
		if serviceID, _, ok := otlpResolver.resolveCanonicalService(ctx, serviceName, namespaceHint); ok {
			return serviceID, true
		}
	}

	if fallbackNS := fallbackMetricServiceNamespace(resource, serviceName); fallbackNS != "" {
		clusterName := resource[semconv.ResourceK8sClusterName]
		if otlpResolver, ok := resolver.(*OTLPEntityResolver); ok && otlpResolver.clusterName != "" {
			clusterName = otlpResolver.clusterName
		}
		return entities.ComputeEntityID(entities.KindService, clusterName, fallbackNS, serviceName), true
	}

	return "", false
}

func fallbackMetricServiceNamespace(resource map[string]string, serviceName string) string {
	namespace := resource[semconv.ResourceK8sNamespaceName]
	if namespace == "" {
		return ""
	}

	resourceService := resource[semconv.ResourceServiceName]
	if resourceService == "" || resourceService == serviceName {
		return namespace
	}
	if resource[semconv.ResourceK8sNodeName] != "" {
		return ""
	}
	return namespace
}

type scalarDataPoint struct {
	Attributes   []*commonpb.KeyValue
	TimeUnixNano uint64
	Value        float64
}

// collectScalarDataPoints extracts scalar (gauge/sum) datapoints from OTLP metrics.
// Histograms are handled separately by the caller so they can emit `_sum` / `_count`
// pairs instead of being collapsed to a single scalar.
func collectScalarDataPoints(metric *metricspb.Metric) []scalarDataPoint {
	if g := metric.GetGauge(); g != nil {
		return numberDataPointsToScalar(g.DataPoints)
	}
	if s := metric.GetSum(); s != nil {
		return numberDataPointsToScalar(s.DataPoints)
	}
	return nil
}

func numberDataPointsToScalar(points []*metricspb.NumberDataPoint) []scalarDataPoint {
	if len(points) == 0 {
		return nil
	}

	scalars := make([]scalarDataPoint, 0, len(points))
	for _, dp := range points {
		var rawValue float64
		switch v := dp.Value.(type) {
		case *metricspb.NumberDataPoint_AsDouble:
			rawValue = v.AsDouble
		case *metricspb.NumberDataPoint_AsInt:
			rawValue = float64(v.AsInt)
		default:
			continue
		}

		scalars = append(scalars, scalarDataPoint{
			Attributes:   dp.Attributes,
			TimeUnixNano: dp.TimeUnixNano,
			Value:        rawValue,
		})
	}

	return scalars
}

// resolveDirectionalName resolves metrics whose semantic identity depends on an
// OTLP attribute, keeping the translated semconv name for all others.
func resolveDirectionalName(metricName string, attrs []*commonpb.KeyValue) string {
	switch metricName {
	case "system.disk.io":
		dir := kvString(attrs, semconv.AttrDirection)
		if dir == "write" {
			return "system.disk.io.write"
		}
		return "system.disk.io.read"
	case "system.network.io":
		dir := kvString(attrs, semconv.AttrDirection)
		if dir == "transmit" || dir == "tx" {
			return "system.network.io.transmit"
		}
		return "system.network.io.receive"
	case "k8s.node.condition":
		condType := kvString(attrs, semconv.AttrCondition)
		switch condType {
		case "Ready":
			return "k8s.node.condition.ready"
		case "MemoryPressure":
			return "k8s.node.condition.memory_pressure"
		case "DiskPressure":
			return "k8s.node.condition.disk_pressure"
		}
		return "" // skip unknown conditions
	}
	return metricName
}

// kvString returns the string value for key from a []*commonpb.KeyValue slice, or "".
func kvString(attrs []*commonpb.KeyValue, key string) string {
	for _, kv := range attrs {
		if kv.Key == key && kv.Value != nil {
			if sv, ok := kv.Value.Value.(*commonpb.AnyValue_StringValue); ok {
				return sv.StringValue
			}
		}
	}
	return ""
}
