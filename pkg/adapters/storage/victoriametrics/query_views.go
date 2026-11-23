package victoriametrics

import (
	"context"
	"fmt"
	"github.com/erickivel/karthos/pkg/backend/query"
	"github.com/erickivel/karthos/pkg/sources"
	"github.com/erickivel/karthos/pkg/sources/contract"
	"strings"
	"time"
)

// RenderQuery exposes the exact production expression for diagnostics and
// independently inspecting API/storage parity; it performs no I/O.
func RenderQuery(q query.MetricQuery) string {
	views := sources.DefaultRegistry().QueryViews(q.MetricName)
	if len(views) == 0 {
		return buildBasePromQL(q)
	}
	q = resolveQueryDefaults(q)
	var expressions []string
	for _, v := range views {
		if q.Source != "" && v.SourceID != q.Source {
			continue
		}
		sub := q
		sub.Source = v.SourceID
		sub.MetricName = v.InputMetric
		for _, b := range v.Branches {
			e := buildBasePromQL(sub, renderMatchers(b.Matchers)...)
			if b.Zero {
				e = "(" + e + ") * 0"
			}
			expressions = append(expressions, "("+e+")")
		}
	}
	return strings.Join(expressions, " or ")
}
func buildPromQL(q query.MetricQuery) string { return RenderQuery(q) }
func renderMatchers(matchers []contract.LabelMatcher) []string {
	result := make([]string, 0, len(matchers))
	for _, m := range matchers {
		result = append(result, fmt.Sprintf(`%s%s"%s"`, escapeLabelName(m.Name), m.Operator, escapeLabelValue(m.Value)))
	}
	return result
}
func escapeLabelName(name string) string {
	var b strings.Builder
	for _, c := range name {
		if c != '_' && (c < 'a' || c > 'z') && (c < 'A' || c > 'Z') && (c < '0' || c > '9') {
			b.WriteByte('\\')
		}
		b.WriteRune(c)
	}
	return b.String()
}

func (s *VMMetricStore) queryViewStats(ctx context.Context, view contract.QueryView, lookback time.Duration) (int64, time.Time, error) {
	var counts, last []string
	q := query.MetricQuery{MetricName: view.InputMetric, Source: view.SourceID}
	for _, branch := range view.Branches {
		selector := metricSelectorWithMatchers(q, renderMatchers(branch.Matchers)...)
		counts = append(counts, fmt.Sprintf("count_over_time(%s[%s])", selector, durationExpr(lookback)))
		last = append(last, fmt.Sprintf("tlast_over_time(%s[%s])", selector, durationExpr(lookback)))
	}
	countRows, err := s.instantQuery(ctx, "sum by (source) ("+strings.Join(counts, " or ")+")")
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("view coverage: %w", err)
	}
	lastRows, err := s.instantQuery(ctx, "max by (source) ("+strings.Join(last, " or ")+")")
	if err != nil {
		return 0, time.Time{}, fmt.Errorf("view freshness: %w", err)
	}
	var count int64
	var latest time.Time
	for _, row := range countRows {
		if row.Source == view.SourceID {
			count += int64(row.Value)
		}
	}
	for _, row := range lastRows {
		if row.Source == view.SourceID {
			at := time.Unix(int64(row.Value), 0).UTC()
			if at.After(latest) {
				latest = at
			}
		}
	}
	return count, latest, nil
}
