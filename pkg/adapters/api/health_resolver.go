package api

import (
	"context"
	"time"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
)

type healthQuality string

const (
	healthQualityComplete    healthQuality = "complete"
	healthQualityPartial     healthQuality = "partial"
	healthQualityUnavailable healthQuality = "unavailable"
	healthLookback                         = 5 * time.Minute
)

type healthSampleResult struct {
	samples []metrics.MetricSample
	quality healthQuality
	err     error
}

// queryHealthSamples keeps snapshot-wide PromQL text constant by omitting the
// entity matcher and intersecting returned recent samples with current graph
// IDs. Single-entity callers may request an exact store-side filter.
func queryHealthSamples(
	ctx context.Context,
	store persistence.MetricStore,
	metricName string,
	ids []entities.EntityID,
	exact bool,
) healthSampleResult {
	if len(ids) == 0 {
		return healthSampleResult{quality: healthQualityComplete}
	}
	if store == nil {
		return healthSampleResult{quality: healthQualityUnavailable}
	}

	now := time.Now().UTC()
	q := query.MetricQuery{MetricName: metricName, From: now.Add(-healthLookback), To: now}
	if exact {
		q.EntityIDs = ids
	}
	samples, err := store.Query(ctx, q)
	if err != nil {
		return healthSampleResult{quality: healthQualityUnavailable, err: err}
	}

	wanted := make(map[entities.EntityID]struct{}, len(ids))
	for _, id := range ids {
		wanted[id] = struct{}{}
	}
	filtered := make([]metrics.MetricSample, 0, len(samples))
	for _, sample := range samples {
		if _, ok := wanted[sample.EntityID]; ok {
			filtered = append(filtered, sample)
		}
	}
	return healthSampleResult{samples: filtered, quality: healthQualityComplete}
}

func combineHealthQuality(qualities ...healthQuality) healthQuality {
	complete := 0
	unavailable := 0
	for _, quality := range qualities {
		switch quality {
		case healthQualityComplete:
			complete++
		case healthQualityUnavailable:
			unavailable++
		}
	}
	if unavailable == 0 {
		return healthQualityComplete
	}
	if complete == 0 {
		return healthQualityUnavailable
	}
	return healthQualityPartial
}
