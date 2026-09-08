package api

import (
	"context"
	"time"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/topology"
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

// HealthResolver derives graph and status health from direct catalog metrics.
// It is intentionally above MetricStore so in-memory and VictoriaMetrics
// backends share exactly the same health semantics.
type HealthResolver struct{ store persistence.MetricStore }

type healthResolution struct {
	Statuses   map[entities.EntityID]string
	Pods       healthQuality
	Containers healthQuality
	Errors     []error
}

func NewHealthResolver(store persistence.MetricStore) *HealthResolver {
	return &HealthResolver{store: store}
}

func (r *HealthResolver) Resolve(ctx context.Context, nodes []topology.Node, exact bool) healthResolution {
	result := healthResolution{Statuses: make(map[entities.EntityID]string)}
	var podIDs, containerIDs []entities.EntityID
	for _, node := range nodes {
		switch node.Kind {
		case entities.KindPod:
			podIDs = append(podIDs, node.ID)
		case entities.KindContainer:
			containerIDs = append(containerIDs, node.ID)
		}
	}
	pods := queryHealthSamples(ctx, r.store, "k8s.pod.phase", podIDs, exact)
	result.Pods = pods.quality
	if pods.err != nil {
		result.Errors = append(result.Errors, pods.err)
	} else {
		for id, value := range latestSamplePerEntity(pods.samples) {
			result.Statuses[id] = podHealthFromPhase(value)
		}
	}
	ready := queryHealthSamples(ctx, r.store, "k8s.container.ready", containerIDs, exact)
	restarts := queryHealthSamplesWithAggregation(ctx, r.store, "k8s.container.restarts", containerIDs, exact, metrics.AggregationWindowRate, healthLookback)
	result.Containers = combineHealthQuality(ready.quality, restarts.quality)
	if ready.err != nil {
		result.Errors = append(result.Errors, ready.err)
	}
	if restarts.err != nil {
		result.Errors = append(result.Errors, restarts.err)
	}
	if result.Containers != healthQualityUnavailable {
		readyValues, restartRates := latestSamplePerEntity(ready.samples), latestSamplePerEntity(restarts.samples)
		for _, id := range containerIDs {
			if restartRates[id] > 0 {
				result.Statuses[id] = metrics.HealthRestarting
				continue
			}
			readyValue, observed := readyValues[id]
			if !observed {
				result.Statuses[id] = metrics.HealthUnknown
				continue
			}
			switch readyValue {
			case 1:
				result.Statuses[id] = metrics.HealthHealthy
			case 0:
				result.Statuses[id] = metrics.HealthRunningNotReady
			}
		}
	}
	return result
}

func podHealthFromPhase(value float64) string {
	switch int(value) {
	case 1:
		return metrics.HealthPending
	case 2:
		return metrics.HealthHealthy
	case 3:
		return metrics.HealthSucceeded
	case 4:
		return metrics.HealthFailed
	default:
		return metrics.HealthUnknown
	}
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
	return queryHealthSamplesWithAggregation(ctx, store, metricName, ids, exact, metrics.AggregationLatest, 0)
}

func queryHealthSamplesWithAggregation(ctx context.Context, store persistence.MetricStore, metricName string, ids []entities.EntityID, exact bool, aggregation metrics.AggregationMode, window time.Duration) healthSampleResult {
	if len(ids) == 0 {
		return healthSampleResult{quality: healthQualityComplete}
	}
	if store == nil {
		return healthSampleResult{quality: healthQualityUnavailable}
	}

	now := time.Now().UTC()
	q := query.MetricQuery{MetricName: metricName, From: now.Add(-healthLookback), To: now, Aggregation: aggregation, Window: window}
	if exact {
		q.EntityIDs = ids
	}
	resolved, err := query.ResolveMetricQuery(q, now)
	if err != nil {
		return healthSampleResult{quality: healthQualityUnavailable, err: err}
	}
	samples, err := store.Query(ctx, resolved.MetricQuery)
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
	partial := 0
	unavailable := 0
	for _, quality := range qualities {
		switch quality {
		case healthQualityComplete:
			complete++
		case healthQualityPartial:
			partial++
		case healthQualityUnavailable:
			unavailable++
		}
	}
	if partial > 0 || (complete > 0 && unavailable > 0) {
		return healthQualityPartial
	}
	if unavailable > 0 {
		return healthQualityUnavailable
	}
	return healthQualityComplete
}
