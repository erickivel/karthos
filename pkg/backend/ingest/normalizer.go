package ingest

import (
	"context"
	"fmt"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/semconv"
	"github.com/erickivel/oksg/pkg/sources"
)

// OTLPNormalizer normalizes OTLP metric envelopes into semconv MetricSamples.
type OTLPNormalizer struct {
	resolver     EntityResolver
	dialect      *metrics.DialectRegistry
	sourcePolicy *MetricSourcePolicy
}

// NormalizerOption configures optional behavior on the OTLPNormalizer.
type NormalizerOption func(*OTLPNormalizer)

// WithAssignment configures assignment enforcement while retaining the built-in
// logical-source registry. Prefer WithSourcePolicy when the policy is shared
// with topology derivation.
func WithAssignment(a metrics.Assignment) NormalizerOption {
	return func(n *OTLPNormalizer) {
		n.sourcePolicy = NewMetricSourcePolicy(sources.DefaultRegistry().MetricIDs(), a)
	}
}

// WithSourcePolicy shares one source policy with the normalizer.
func WithSourcePolicy(policy *MetricSourcePolicy) NormalizerOption {
	return func(n *OTLPNormalizer) {
		if policy != nil {
			n.sourcePolicy = policy
		}
	}
}

// NewOTLPNormalizer creates a new OTLPNormalizer backed by the given EntityResolver.
func NewOTLPNormalizer(resolver EntityResolver, dialect *metrics.DialectRegistry, opts ...NormalizerOption) *OTLPNormalizer {
	if resolver == nil {
		panic("ingest.NewOTLPNormalizer: resolver is required")
	}
	if dialect == nil {
		panic("ingest.NewOTLPNormalizer: dialect registry is required")
	}
	n := &OTLPNormalizer{
		resolver:     resolver,
		dialect:      dialect,
		sourcePolicy: NewMetricSourcePolicy(sources.DefaultRegistry().MetricIDs(), nil),
	}
	for _, opt := range opts {
		opt(n)
	}
	return n
}

// DroppedSampleCount returns the total number of provenance-policy rejections.
func (n *OTLPNormalizer) DroppedSampleCount() uint64 {
	return n.sourcePolicy.DroppedSampleCount()
}

// DroppedSampleCountForReason returns the number of rejections for one reason.
func (n *OTLPNormalizer) DroppedSampleCountForReason(reason SourceRejectionReason) uint64 {
	return n.sourcePolicy.DroppedSampleCountForReason(reason)
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
					translatedName := n.dialect.TranslateForSource(firstKVString(dp.Attributes, semconv.AttrOKSGSource, semconv.AttrOKSGSourceProm), metric.Name)
					mapping, ok := metrics.OTLPSemconvMetrics[translatedName]
					if !ok {
						continue
					}
					validation := n.sourcePolicy.Validate(translatedName, dp.Attributes)
					if !validation.Accepted() {
						n.sourcePolicy.RecordRejection(validation)
						continue
					}
					if dp.Count == 0 || dp.Sum == nil {
						continue
					}
					plan := newNormalizationPlan(metric.Name, translatedName, validation.Source, envelope.Resource, dp.Attributes, mapping)
					if err := applySourceNormalization(ctx, n.resolver, &plan); err != nil {
						return nil, fmt.Errorf("apply %s source module: %w", plan.Source, err)
					}
					if plan.Skip || plan.MetricName == "" {
						continue
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
				translatedName := n.dialect.TranslateForSource(firstKVString(dp.Attributes, semconv.AttrOKSGSource, semconv.AttrOKSGSourceProm), metric.Name)
				mapping, ok := metrics.OTLPSemconvMetrics[translatedName]
				if !ok {
					continue
				}
				ts := time.Unix(0, int64(dp.TimeUnixNano)).UTC() //#nosec G115 -- Unix nanosecond timestamps from real systems won't overflow int64 before year 2262
				validation := n.sourcePolicy.Validate(translatedName, dp.Attributes)
				if !validation.Accepted() {
					n.sourcePolicy.RecordRejection(validation)
					continue
				}
				plan := newNormalizationPlan(metric.Name, translatedName, validation.Source, envelope.Resource, dp.Attributes, mapping)
				if err := applySourceNormalization(ctx, n.resolver, &plan); err != nil {
					return nil, fmt.Errorf("apply %s source module: %w", plan.Source, err)
				}
				if plan.Skip || plan.MetricName == "" {
					continue
				}
				value := dp.Value * plan.ConversionFactor
				if usesDeltaCounterSemantics(mapping, dp.AggregationTemporality) {
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
					samples = append(samples, metrics.MetricSample{
						MetricName: derived.MetricName,
						EntityID:   plan.EntityID,
						Value:      dp.Value * derived.ConversionFactor,
						Timestamp:  ts,
						Labels:     cloneLabels(plan.Labels),
						Unit:       derived.Unit,
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
	mapping metrics.OTLPMetricMapping,
	temporality metricspb.AggregationTemporality,
) bool {
	if mapping.Kind != metrics.MetricKindCounter {
		return false
	}
	return temporality == metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA
}

func usesDeltaHistogramSemantics(temporality metricspb.AggregationTemporality) bool {
	return temporality == metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA
}
