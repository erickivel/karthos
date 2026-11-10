package ingest

import (
	"cmp"
	"math"
	"slices"
	"sync"
	"time"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"
	"google.golang.org/protobuf/proto"

	"github.com/erickivel/karthos/pkg/core/topology"
)

type callSeriesKey struct {
	metric, source, resource, attributes string
}

type callMetricPoint struct {
	attrs       []*commonpb.KeyValue
	at, start   uint64
	value       callCount
	temporality metricspb.AggregationTemporality
}

type callObservation struct {
	key   callSeriesKey
	point callMetricPoint
	edge  topology.Edge
}

func compareCallObservationTime(a, b callObservation) int {
	return cmp.Compare(a.point.at, b.point.at)
}

type callSeriesState struct {
	at, start   uint64
	value       callCount
	temporality metricspb.AggregationTemporality
	seenAt      time.Time
}

type callSeriesTracker struct {
	mu        sync.Mutex
	startedAt time.Time
	now       func() time.Time
	nextPrune time.Time
	entries   map[callSeriesKey]callSeriesState
}

// increases evaluates an already time-ordered envelope atomically with respect
// to other envelopes. Its output keeps the order of the input observations.
func (t *callSeriesTracker) increases(observations []callObservation) []callObservation {
	t.mu.Lock()
	defer t.mu.Unlock()

	now := t.now()
	cutoff := now.Add(-topology.HistoryRetention)
	if !now.Before(t.nextPrune) {
		for key, state := range t.entries {
			if state.seenAt.Before(cutoff) {
				delete(t.entries, key)
			}
		}
		t.nextPrune = now.Add(time.Minute)
	}

	startFloor := t.startedAt
	if cutoff.After(startFloor) {
		startFloor = cutoff
	}
	increases := observations[:0]
	for _, observation := range observations {
		point := observation.point
		previous, exists := t.entries[observation.key]
		if exists && previous.seenAt.Before(cutoff) {
			exists = false
		}
		if exists && point.at <= previous.at {
			continue
		}
		t.entries[observation.key] = callSeriesState{
			at: point.at, start: point.start, value: point.value,
			temporality: point.temporality, seenAt: now,
		}
		if point.increased(previous, exists, startFloor) {
			increases = append(increases, observation)
		}
	}
	return increases
}

func (p callMetricPoint) increased(previous callSeriesState, exists bool, startFloor time.Time) bool {
	if !p.value.positive() {
		return false
	}
	if p.temporality == metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA {
		return true
	}
	if p.start == p.at {
		return false // OTLP unknown start: the value is a baseline.
	}
	if !exists || previous.temporality != p.temporality {
		return p.start != 0 && time.Unix(0, int64(p.start)).After(startFloor) // #nosec G115 -- start is bounded by the validated end timestamp
	}
	// A lower positive value is a reset. A newer explicit start can also
	// identify a reset when the count has already reached its previous value.
	return p.value.compare(previous.value) != 0 ||
		(p.start != 0 && p.start != previous.start && p.start >= previous.at)
}

func callMetricPoints(metric *metricspb.Metric) []callMetricPoint {
	var points []callMetricPoint
	if sum := metric.GetSum(); sum != nil && sum.IsMonotonic && callTemporalitySupported(sum.AggregationTemporality) {
		for _, dp := range sum.DataPoints {
			if !validCallTime(dp.TimeUnixNano, dp.StartTimeUnixNano, dp.Flags) {
				continue
			}
			value, ok := numberCallCount(dp)
			if ok {
				points = append(points, callMetricPoint{dp.Attributes, dp.TimeUnixNano, dp.StartTimeUnixNano, value, sum.AggregationTemporality})
			}
		}
	}
	if histogram := metric.GetHistogram(); histogram != nil && callTemporalitySupported(histogram.AggregationTemporality) {
		for _, dp := range histogram.DataPoints {
			if validCallTime(dp.TimeUnixNano, dp.StartTimeUnixNano, dp.Flags) {
				points = append(points, callMetricPoint{
					dp.Attributes, dp.TimeUnixNano, dp.StartTimeUnixNano, callCount{integer: dp.Count}, histogram.AggregationTemporality,
				})
			}
		}
	}
	return points
}

func callTemporalitySupported(temporality metricspb.AggregationTemporality) bool {
	return temporality == metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE ||
		temporality == metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_DELTA
}

func validCallTime(at, start uint64, flags uint32) bool {
	return at > 0 && at <= math.MaxInt64 && start <= at &&
		flags&uint32(metricspb.DataPointFlags_DATA_POINT_FLAGS_NO_RECORDED_VALUE_MASK) == 0
}

// Keep integer counters and histogram counts exact above the float64 precision
// limit. Double counters retain the precision supplied by their producer.
type callCount struct {
	integer uint64
	decimal float64
	isFloat bool
}

func numberCallCount(point *metricspb.NumberDataPoint) (callCount, bool) {
	switch value := point.Value.(type) {
	case *metricspb.NumberDataPoint_AsInt:
		if value.AsInt >= 0 {
			return callCount{integer: uint64(value.AsInt)}, true
		}
	case *metricspb.NumberDataPoint_AsDouble:
		if value.AsDouble >= 0 && !math.IsInf(value.AsDouble, 0) {
			return callCount{decimal: value.AsDouble, isFloat: true}, true
		}
	}
	return callCount{}, false
}

func (c callCount) positive() bool { return c.integer > 0 || c.decimal > 0 }

func (c callCount) compare(other callCount) int {
	switch {
	case c.isFloat && other.isFloat:
		return cmp.Compare(c.decimal, other.decimal)
	case c.isFloat:
		return compareDecimalToCount(c.decimal, other.integer)
	case other.isFloat:
		return -compareDecimalToCount(other.decimal, c.integer)
	default:
		return cmp.Compare(c.integer, other.integer)
	}
}

func compareDecimalToCount(decimal float64, count uint64) int {
	if decimal >= 1<<64 {
		return 1
	}
	integer, fraction := math.Modf(decimal)
	if comparison := cmp.Compare(uint64(integer), count); comparison != 0 {
		return comparison
	}
	return cmp.Compare(fraction, 0)
}

func callResourceKey(attrs []*commonpb.KeyValue, fallback map[string]string) string {
	if len(attrs) == 0 {
		for key, value := range fallback {
			attrs = append(attrs, &commonpb.KeyValue{Key: key, Value: &commonpb.AnyValue{
				Value: &commonpb.AnyValue_StringValue{StringValue: value},
			}})
		}
	}
	return callAttributeKey(attrs)
}

func callAttributeKey(attrs []*commonpb.KeyValue) string {
	ordered := slices.Clone(attrs)
	slices.SortFunc(ordered, func(a, b *commonpb.KeyValue) int { return cmp.Compare(a.GetKey(), b.GetKey()) })
	// Values come from a validated protojson payload. Encoding the typed values
	// keeps e.g. the string "1" distinct from the integer 1.
	encoded, _ := proto.MarshalOptions{Deterministic: true}.Marshal(&commonpb.KeyValueList{Values: ordered})
	return string(encoded)
}
