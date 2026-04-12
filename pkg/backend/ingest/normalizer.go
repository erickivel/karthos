package ingest

import (
	"context"
	"fmt"
	"strings"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"

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
func (n *OTLPNormalizer) Normalize(ctx context.Context, envelope schema.RawEnvelope) ([]metrics.MetricSample, error) {
	var rm metricspb.ResourceMetrics
	if err := protojson.Unmarshal(envelope.Payload, &rm); err != nil {
		return nil, fmt.Errorf("unmarshal ResourceMetrics: %w", err)
	}

	entityID, err := n.resolver.Resolve(ctx, envelope.Resource)
	if err != nil {
		return nil, fmt.Errorf("resolve entity: %w", err)
	}

	var samples []metrics.MetricSample

	for _, sm := range rm.ScopeMetrics {
		for _, metric := range sm.Metrics {
			mapping, ok := metrics.OTLPToCanonicalMappings[metric.Name]
			if !ok {
				continue
			}

			dps := collectDataPoints(metric)
			for _, dp := range dps {
				var rawValue float64
				switch v := dp.Value.(type) {
				case *metricspb.NumberDataPoint_AsDouble:
					rawValue = v.AsDouble
				case *metricspb.NumberDataPoint_AsInt:
					rawValue = float64(v.AsInt)
				default:
					continue
				}

				ts := time.Unix(0, int64(dp.TimeUnixNano)).UTC() //#nosec G115 -- Unix nanosecond timestamps from real systems won't overflow int64 before year 2262
				value := rawValue * mapping.ConversionFactor

				canonicalName := resolveDirectionalName(metric.Name, mapping.CanonicalName, dp.Attributes)
				if canonicalName == "" {
					continue
				}

				labels := map[string]string{"source": envelope.Source}

				// Preserve edge context from servicegraph connector metrics.
				if strings.HasPrefix(metric.Name, "traces.service.graph.") {
					if c := kvString(dp.Attributes, "client"); c != "" {
						labels["edge_client"] = c
					}
					if s := kvString(dp.Attributes, "server"); s != "" {
						labels["edge_server"] = s
					}
				}

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

// collectDataPoints returns all NumberDataPoints from Gauge or Sum metrics.
func collectDataPoints(metric *metricspb.Metric) []*metricspb.NumberDataPoint {
	if g := metric.GetGauge(); g != nil {
		return g.DataPoints
	}
	if s := metric.GetSum(); s != nil {
		return s.DataPoints
	}
	return nil
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
