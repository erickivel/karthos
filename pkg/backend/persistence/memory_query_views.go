package persistence

import (
	"github.com/erickivel/karthos/pkg/backend/query"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/sources/contract"
	"regexp"
	"time"
)

// InMemoryMetricStoreOption configures optional source-owned query interpretations.
type InMemoryMetricStoreOption func(*InMemoryMetricStore)

// WithMetricQueryViews injects the compiled view registry without making the
// persistence package depend on source module implementations.
func WithMetricQueryViews(views func(string) []contract.QueryView) InMemoryMetricStoreOption {
	return func(store *InMemoryMetricStore) { store.queryViews = views }
}

func matchesViewBranch(labels map[string]string, matchers []contract.LabelMatcher) bool {
	for _, matcher := range matchers {
		value := labels[matcher.Name]
		switch matcher.Operator {
		case "=":
			if value != matcher.Value {
				return false
			}
		case "!=":
			if value == matcher.Value {
				return false
			}
		case "=~", "!~":
			expression, err := regexp.Compile("^(?:" + matcher.Value + ")$")
			if err != nil {
				return false
			}
			matches := expression.MatchString(value)
			if matcher.Operator == "=~" && !matches || matcher.Operator == "!~" && matches {
				return false
			}
		default:
			return false
		}
	}
	return true
}

// queryViewSamples selects original partitions before the existing counter
// rollups. Zero branches change only the resulting value, never source samples.
// Caller holds the store read lock.
func (s *InMemoryMetricStore) queryViewSamples(q query.MetricQuery) ([]metrics.MetricSample, bool) {
	if s.queryViews == nil {
		return nil, false
	}
	views := s.queryViews(q.MetricName)
	if len(views) == 0 {
		return nil, false
	}
	var result []metrics.MetricSample
	seen := map[string]bool{}
	for _, view := range views {
		if q.Source != "" && q.Source != view.SourceID {
			continue
		}
		sub := q
		sub.MetricName = view.InputMetric
		sub.Source = view.SourceID
		if sub.Step == 0 && (sub.Aggregation == "" || sub.Aggregation == metrics.AggregationLatest) {
			if sub.To.IsZero() {
				sub.To = s.nowFn()
			}
			sub.From = sub.To
			sub.Step = time.Second
		}
		for _, branch := range view.Branches {
			var selected []metrics.MetricSample
			for _, sample := range s.samples {
				if sample.MetricName == view.InputMetric && sample.Labels["source"] == view.SourceID && matchesViewBranch(sample.Labels, branch.Matchers) {
					selected = append(selected, sample)
				}
			}
			temporary := InMemoryMetricStore{samples: selected, nowFn: s.nowFn}
			for _, sample := range temporary.queryRawLocked(sub) {
				sample = cloneMetricSample(sample)
				sample.MetricName = q.MetricName
				if branch.Zero {
					sample.Value = 0
				}
				metadata := view.Derivation
				sample.Derivation = &metadata
				key := memorySeriesKey(sample) + sample.Timestamp.UTC().Format(time.RFC3339Nano)
				if seen[key] {
					continue
				}
				seen[key] = true
				result = append(result, sample)
			}
		}
	}
	return result, true
}

// catalogViewSamples replaces materialized view coverage with the classified
// original input observations. Native views keep their existing sample meaning.
func (s *InMemoryMetricStore) catalogViewSamples() []metrics.MetricSample {
	if s.queryViews == nil {
		return s.samples
	}
	replaced := map[string]bool{}
	var projected []metrics.MetricSample
	for _, metric := range metrics.DefaultCatalog() {
		for _, view := range s.queryViews(metric.Name) {
			replaced[view.MetricName+"|"+view.SourceID] = true
			for _, sample := range s.samples {
				if sample.MetricName != view.InputMetric || sample.Labels["source"] != view.SourceID {
					continue
				}
				for _, branch := range view.Branches {
					if matchesViewBranch(sample.Labels, branch.Matchers) {
						sample.MetricName = view.MetricName
						projected = append(projected, sample)
						break
					}
				}
			}
		}
	}
	for _, sample := range s.samples {
		if !replaced[sample.MetricName+"|"+sample.Labels["source"]] {
			projected = append(projected, sample)
		}
	}
	return projected
}
