package persistence

import (
	"context"

	"github.com/erickivel/karthos/pkg/backend/query"
	"github.com/erickivel/karthos/pkg/core/metrics"
)

// DecorateMetricStore applies provider-neutral public output derivations to a
// metric store. Storage itself retains only canonical numeric samples.
func DecorateMetricStore(store MetricStore) MetricStore {
	if store == nil {
		return nil
	}
	if _, decorated := store.(metricStoreDecorator); decorated {
		return store
	}
	return metricStoreDecorator{MetricStore: store}
}

type metricStoreDecorator struct{ MetricStore }

func (s metricStoreDecorator) Query(ctx context.Context, q query.MetricQuery) ([]metrics.MetricSample, error) {
	samples, err := s.MetricStore.Query(ctx, q)
	if err != nil {
		return nil, err
	}
	decorated := make([]metrics.MetricSample, len(samples))
	for i := range samples {
		decorated[i] = cloneMetricSample(samples[i])
		if metrics.IsHealthStatusMetric(decorated[i].MetricName) {
			decorated[i].ValueLabel = metrics.HealthLabel(decorated[i].MetricName, decorated[i].Value)
		}
	}
	return decorated, nil
}
