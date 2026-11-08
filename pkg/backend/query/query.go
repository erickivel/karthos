package query

import (
	"fmt"
	"time"

	"github.com/erickivel/karthos/pkg/core/entities"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/core/topology"
)

type MetricQuery struct {
	Source       string
	EntityIDs    []entities.EntityID
	MetricName   string
	From         time.Time
	To           time.Time
	EdgeSourceID entities.EntityID
	EdgeTargetID entities.EntityID
	Aggregation  metrics.AggregationMode
	Window       time.Duration
	Unit         metrics.Unit
	// Step and MaxSampleAge are populated by ResolveMetricQuery for public
	// evaluated trend reads. A zero Step keeps direct in-process reads raw.
	Step         time.Duration
	MaxSampleAge time.Duration
}

const (
	DefaultMetricRange  = time.Hour
	DefaultMetricStep   = 15 * time.Second
	DefaultMaxSampleAge = 5 * time.Minute
)

// ResolvedMetricQuery is the provider-neutral trend query passed to metric
// storage. Its timestamps identify evaluation points, not producer timestamps.
type ResolvedMetricQuery struct {
	MetricQuery
}

// ResolveMetricQuery applies the fixed public trend-view defaults. When only
// To is supplied, the default range ends at To; when only From is supplied,
// the range ends at now.
func ResolveMetricQuery(q MetricQuery, now time.Time) (ResolvedMetricQuery, error) {
	if now.IsZero() {
		now = time.Now().UTC()
	}
	from, to := q.From, q.To
	if to.IsZero() {
		to = now
	}
	if from.IsZero() {
		from = to.Add(-DefaultMetricRange)
	}
	if from.After(to) {
		return ResolvedMetricQuery{}, fmt.Errorf("from must not be after to")
	}
	q.From, q.To = from.UTC(), to.UTC()
	q.Step, q.MaxSampleAge = DefaultMetricStep, DefaultMaxSampleAge
	return ResolvedMetricQuery{
		MetricQuery: q,
	}, nil
}

// EvaluationPoints returns the inclusive points on the query's fixed trend
// grid. The final To point is included even when it is not step-aligned.
func (q ResolvedMetricQuery) EvaluationPoints() []time.Time {
	if q.Step <= 0 || q.From.IsZero() || q.To.IsZero() {
		return nil
	}
	points := make([]time.Time, 0, int(q.To.Sub(q.From)/q.Step)+2)
	for point := q.From; !point.After(q.To); point = point.Add(q.Step) {
		points = append(points, point)
	}
	if points[len(points)-1] != q.To {
		points = append(points, q.To)
	}
	return points
}

type EdgeQuery struct {
	Source   entities.EntityID
	Target   entities.EntityID
	Relation topology.RelationType
}

// FilterEdges returns the edges that satisfy every non-empty filter in q.
func FilterEdges(edges []topology.Edge, q EdgeQuery) []topology.Edge {
	result := make([]topology.Edge, 0, len(edges))
	for _, edge := range edges {
		if q.Source != "" && edge.Source != q.Source {
			continue
		}
		if q.Target != "" && edge.Target != q.Target {
			continue
		}
		if q.Relation != "" && edge.Relation != q.Relation {
			continue
		}
		result = append(result, edge)
	}
	return result
}
