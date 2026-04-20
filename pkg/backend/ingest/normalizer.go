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
)

// OTLPNormalizer normalizes OTLP metric envelopes into canonical MetricSamples.
type OTLPNormalizer struct {
	resolver EntityResolver
}

// NewOTLPNormalizer creates a new OTLPNormalizer backed by the given EntityResolver.
func NewOTLPNormalizer(resolver EntityResolver) *OTLPNormalizer {
	return &OTLPNormalizer{resolver: resolver}
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
			mapping, ok := metrics.OTLPToCanonicalMappings[metric.Name]
			if !ok {
				continue
			}

			// Histograms are decomposed into _sum / _count pairs.
			if h := metric.GetHistogram(); h != nil {
				for _, dp := range h.DataPoints {
					if dp.Count == 0 || dp.Sum == nil {
						continue
					}
					canonicalName := resolveDirectionalName(metric.Name, mapping.CanonicalName, dp.Attributes)
					if canonicalName == "" {
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
							MetricName: canonicalName + "_sum",
							EntityID:   entityID,
							Value:      *dp.Sum * mapping.ConversionFactor,
							Timestamp:  ts,
							Labels:     labels,
						},
						metrics.MetricSample{
							MetricName: canonicalName + "_count",
							EntityID:   entityID,
							Value:      float64(dp.Count),
							Timestamp:  ts,
							Labels:     cloneLabels(labels),
						},
					)
				}
				continue
			}

			dps := collectScalarDataPoints(metric)
			for _, dp := range dps {
				ts := time.Unix(0, int64(dp.TimeUnixNano)).UTC() //#nosec G115 -- Unix nanosecond timestamps from real systems won't overflow int64 before year 2262
				value := dp.Value * mapping.ConversionFactor

				canonicalName := resolveDirectionalName(metric.Name, mapping.CanonicalName, dp.Attributes)
				if canonicalName == "" {
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
					MetricName: canonicalName,
					EntityID:   entityID,
					Value:      value,
					Timestamp:  ts,
					Labels:     labels,
				})
			}
		}
	}

	return samples, nil
}

func baseLabels(source string) map[string]string {
	return map[string]string{"source": source}
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
	if c := kvString(attrs, "client"); c != "" {
		labels["edge_client"] = c
		if clientID, ok := resolveServiceMetricEntityID(ctx, resolver, resource, c, kvString(attrs, "client_namespace")); ok {
			labels["edge_client_id"] = string(clientID)
		}
	}
	if s := kvString(attrs, "server"); s != "" {
		labels["edge_server"] = s
		if serverID, ok := resolveServiceMetricEntityID(ctx, resolver, resource, s, kvString(attrs, "server_namespace")); ok {
			labels["edge_server_id"] = string(serverID)
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
		client := kvString(attrs, "client")
		if client == "" {
			return "", true, nil
		}
		if clientID, ok := resolveServiceMetricEntityID(ctx, n.resolver, resource, client, kvString(attrs, "client_namespace")); ok {
			return clientID, false, nil
		}
		return "", true, nil
	}

	if strings.HasPrefix(metricName, "traces.span.metrics.") {
		serviceName := resource["service.name"]
		if serviceName == "" {
			return "", true, nil
		}
		if serviceID, ok := resolveServiceMetricEntityID(ctx, n.resolver, resource, serviceName, resource["k8s.namespace.name"]); ok {
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
		clusterName := resource["k8s.cluster.name"]
		if otlpResolver, ok := resolver.(*OTLPEntityResolver); ok && otlpResolver.clusterName != "" {
			clusterName = otlpResolver.clusterName
		}
		return entities.ComputeEntityID(entities.KindService, clusterName, fallbackNS, serviceName), true
	}

	return "", false
}

func fallbackMetricServiceNamespace(resource map[string]string, serviceName string) string {
	namespace := resource["k8s.namespace.name"]
	if namespace == "" {
		return ""
	}

	resourceService := resource["service.name"]
	if resourceService == "" || resourceService == serviceName {
		return namespace
	}
	if resource["k8s.node.name"] != "" {
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

// resolveDirectionalName overrides the default canonical name based on the "direction"
// attribute for disk and network metrics.
func resolveDirectionalName(metricName, defaultName string, attrs []*commonpb.KeyValue) string {
	switch metricName {
	case "system.disk.io":
		dir := kvString(attrs, "direction")
		if dir == "write" {
			return "disk_write_bps"
		}
		return "disk_read_bps"
	case "system.network.io":
		dir := kvString(attrs, "direction")
		if dir == "transmit" || dir == "tx" {
			return "net_tx_bps"
		}
		return "net_rx_bps"
	case "k8s.node.condition":
		condType := kvString(attrs, "condition")
		switch condType {
		case "Ready":
			return "node_condition_ready"
		case "MemoryPressure":
			return "node_condition_memory_pressure"
		case "DiskPressure":
			return "node_condition_disk_pressure"
		}
		return "" // skip unknown conditions
	}
	return defaultName
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
