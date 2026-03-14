package registry

import "github.com/erickivel/oksg/pkg/core/metrics"

type MetricRegistry interface {
	Register(metric metrics.CanonicalMetric) error
	Get(name string) (metrics.CanonicalMetric, bool)
	List() []metrics.CanonicalMetric
}

type StaticMetricRegistry struct {
	metrics map[string]metrics.CanonicalMetric
}

func NewStaticMetricRegistry(seed []metrics.CanonicalMetric) *StaticMetricRegistry {
	items := make(map[string]metrics.CanonicalMetric, len(seed))
	for _, metric := range seed {
		items[metric.Name] = metric
	}

	return &StaticMetricRegistry{metrics: items}
}

func (r *StaticMetricRegistry) Register(metric metrics.CanonicalMetric) error {
	if r.metrics == nil {
		r.metrics = make(map[string]metrics.CanonicalMetric)
	}
	r.metrics[metric.Name] = metric
	return nil
}

func (r *StaticMetricRegistry) Get(name string) (metrics.CanonicalMetric, bool) {
	metric, ok := r.metrics[name]
	return metric, ok
}

func (r *StaticMetricRegistry) List() []metrics.CanonicalMetric {
	out := make([]metrics.CanonicalMetric, 0, len(r.metrics))
	for _, metric := range r.metrics {
		out = append(out, metric)
	}
	return out
}
