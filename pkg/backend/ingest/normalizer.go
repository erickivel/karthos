package ingest

import (
	"context"
	"fmt"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/core/schema"
	"github.com/erickivel/karthos/pkg/sources"
	"github.com/erickivel/karthos/pkg/sources/contract"
)

// OTLPNormalizer normalizes OTLP metric envelopes into semconv MetricSamples.
type OTLPNormalizer struct {
	resolver     EntityResolver
	sourcePolicy *MetricSourcePolicy
	modules      sources.Registry
	catalog      map[string]metrics.CanonicalMetric
}

// NormalizerOption configures optional behavior on the OTLPNormalizer.
type NormalizerOption func(*OTLPNormalizer)

// WithSourcePolicy shares one source policy with the normalizer.
func WithSourcePolicy(policy *MetricSourcePolicy) NormalizerOption {
	return func(n *OTLPNormalizer) {
		if policy != nil {
			n.sourcePolicy = policy
		}
	}
}

// NewOTLPNormalizer creates a new OTLPNormalizer backed by the given EntityResolver.
func NewOTLPNormalizer(resolver EntityResolver, modules sources.Registry, opts ...NormalizerOption) *OTLPNormalizer {
	if resolver == nil {
		panic("ingest.NewOTLPNormalizer: resolver is required")
	}
	if len(modules.MetricIDs()) == 0 {
		panic("ingest.NewOTLPNormalizer: source registry is required")
	}
	n := &OTLPNormalizer{
		resolver:     resolver,
		modules:      modules,
		catalog:      metrics.CatalogByName(),
		sourcePolicy: NewMetricSourcePolicy(modules.MetricIDs(), nil),
	}
	for _, opt := range opts {
		opt(n)
	}
	return n
}

// Normalize deserializes a protojson-encoded ResourceMetrics from the envelope payload,
// resolves the entity, and maps each recognized metric to a canonical MetricSample.
//
// For histogram-typed metrics the raw `sum` and `count` are emitted as separate
// samples (with suffixes `_sum` and `_count`). Delta histograms retain their
// incremental values and carry temporality=delta so query-time consumers can derive
// a windowed mean without process-local state.
func (n *OTLPNormalizer) Normalize(ctx context.Context, envelope schema.RawEnvelope) ([]metrics.MetricSample, error) {
	var rm metricspb.ResourceMetrics
	if err := protojson.Unmarshal(envelope.Payload, &rm); err != nil {
		return nil, fmt.Errorf("unmarshal ResourceMetrics: %w", err)
	}

	var samples []metrics.MetricSample

	for _, sm := range rm.ScopeMetrics {
		for _, metric := range sm.Metrics {
			// Histograms are decomposed into _sum / _count pairs.
			if h := metric.GetHistogram(); h != nil {
				for _, dp := range h.DataPoints {
					if dp.TimeUnixNano == 0 {
						continue
					}
					parsed, definition, validation, ok := n.parseAndValidate(metric.Name, dp.Attributes)
					if !ok {
						continue
					}
					if definition.Instrument != metrics.InstrumentHistogram {
						continue
					}
					if dp.Count == 0 || dp.Sum == nil {
						continue
					}
					plan := newNormalizationPlan(metric.Name, parsed.CanonicalName, validation.Source, envelope.Resource, dp.Attributes, parsed)
					if plan.Unit == "" {
						plan.Unit = definition.StoredUnit
					}
					if err := applySourceNormalization(ctx, n.modules, n.resolver, &plan); err != nil {
						return nil, fmt.Errorf("apply %s source module: %w", plan.Source, err)
					}
					if plan.Skip || plan.MetricName == "" {
						continue
					}
					if plan.MetricName != parsed.CanonicalName {
						return nil, fmt.Errorf("source module %q changed parsed metric name %q", plan.Source, parsed.CanonicalName)
					}

					ts := time.Unix(0, int64(dp.TimeUnixNano)).UTC() //#nosec G115 -- Unix nanosecond timestamps from real systems won't overflow int64 before year 2262
					sumValue := *dp.Sum * plan.ConversionFactor
					countValue := float64(dp.Count)
					if usesDeltaHistogramSemantics(h.AggregationTemporality) {
						plan.Labels["temporality"] = "delta"
					}

					samples = append(samples,
						metrics.MetricSample{
							MetricName: plan.MetricName + "_sum",
							EntityID:   plan.EntityID,
							Value:      sumValue,
							Timestamp:  ts,
							Labels:     plan.Labels,
							Unit:       plan.Unit,
						},
						metrics.MetricSample{
							MetricName: plan.MetricName + "_count",
							EntityID:   plan.EntityID,
							Value:      countValue,
							Timestamp:  ts,
							Labels:     cloneLabels(plan.Labels),
							Unit:       metrics.UnitCount,
						},
					)
				}
				continue
			}

			dps := collectScalarDataPoints(metric)
			for _, dp := range dps {
				if dp.TimeUnixNano == 0 {
					continue
				}
				parsed, definition, validation, ok := n.parseAndValidate(metric.Name, dp.Attributes)
				if !ok {
					continue
				}
				if definition.Instrument == metrics.InstrumentHistogram {
					continue
				}
				ts := time.Unix(0, int64(dp.TimeUnixNano)).UTC() //#nosec G115 -- Unix nanosecond timestamps from real systems won't overflow int64 before year 2262
				plan := newNormalizationPlan(metric.Name, parsed.CanonicalName, validation.Source, envelope.Resource, dp.Attributes, parsed)
				if plan.Unit == "" {
					plan.Unit = definition.StoredUnit
				}
				if err := applySourceNormalization(ctx, n.modules, n.resolver, &plan); err != nil {
					return nil, fmt.Errorf("apply %s source module: %w", plan.Source, err)
				}
				if plan.Skip || plan.MetricName == "" {
					continue
				}
				if plan.MetricName != parsed.CanonicalName {
					return nil, fmt.Errorf("source module %q changed parsed metric name %q", plan.Source, parsed.CanonicalName)
				}
				value := dp.Value * plan.ConversionFactor
				if usesDeltaCounterSemantics(definition.Instrument, dp.AggregationTemporality) {
					plan.Labels["temporality"] = "delta"
				}

				samples = append(samples, metrics.MetricSample{
					MetricName: plan.MetricName,
					EntityID:   plan.EntityID,
					Value:      value,
					Timestamp:  ts,
					Labels:     plan.Labels,
					Unit:       plan.Unit,
				})
				for _, derived := range plan.Derived {
					derivedDefinition, accepted := n.catalog[derived.MetricName]
					if !accepted || !n.sourcePolicy.Validate(derived.MetricName, dp.Attributes).Accepted() {
						continue
					}
					samples = append(samples, metrics.MetricSample{
						MetricName: derived.MetricName,
						EntityID:   plan.EntityID,
						Value:      dp.Value * derived.ConversionFactor,
						Timestamp:  ts,
						Labels:     cloneLabels(plan.Labels),
						Unit:       derivedDefinition.StoredUnit,
					})
				}
			}
		}
	}

	return samples, nil
}

func cloneLabels(in map[string]string) map[string]string {
	out := make(map[string]string, len(in))
	for k, v := range in {
		out[k] = v
	}
	return out
}

type scalarDataPoint struct {
	Attributes             []*commonpb.KeyValue
	TimeUnixNano           uint64
	Value                  float64
	AggregationTemporality metricspb.AggregationTemporality
}

// collectScalarDataPoints extracts scalar (gauge/sum) datapoints from OTLP metrics.
// Histograms are handled separately by the caller so they can emit `_sum` / `_count`
// pairs instead of being collapsed to a single scalar.
func collectScalarDataPoints(metric *metricspb.Metric) []scalarDataPoint {
	if g := metric.GetGauge(); g != nil {
		return numberDataPointsToScalar(g.DataPoints, metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_UNSPECIFIED)
	}
	if s := metric.GetSum(); s != nil {
		return numberDataPointsToScalar(s.DataPoints, s.AggregationTemporality)
	}
	return nil
}

func numberDataPointsToScalar(
	points []*metricspb.NumberDataPoint,
	temporality metricspb.AggregationTemporality,
) []scalarDataPoint {
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
			Attributes:             dp.Attributes,
			TimeUnixNano:           dp.TimeUnixNano,
			Value:                  rawValue,
			AggregationTemporality: temporality,
		})
	}

	return scalars
}

func usesDeltaCounterSemantics(
	instrument metrics.MetricInstrument,
	temporality metricspb.AggregationTemporality,
) bool {
	if instrument != metrics.InstrumentCounter {
		return false
	}
	return temporality == metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA
}

func (n *OTLPNormalizer) parseAndValidate(rawMetricName string, attrs []*commonpb.KeyValue) (contract.ParsedMetric, metrics.CanonicalMetric, SourceValidation, bool) {
	parsed, ok := n.modules.ParseMetric(sourceFromAttrs(attrs), rawMetricName, contract.AttributeStrings(attrs))
	if !ok {
		return contract.ParsedMetric{}, metrics.CanonicalMetric{}, SourceValidation{}, false
	}
	definition, ok := n.catalog[parsed.CanonicalName]
	if !ok {
		return contract.ParsedMetric{}, metrics.CanonicalMetric{}, SourceValidation{}, false
	}
	validation := n.sourcePolicy.Validate(parsed.CanonicalName, attrs)
	if !validation.Accepted() {
		return contract.ParsedMetric{}, metrics.CanonicalMetric{}, SourceValidation{}, false
	}
	return parsed, definition, validation, true
}

func usesDeltaHistogramSemantics(temporality metricspb.AggregationTemporality) bool {
	return temporality == metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA
}
