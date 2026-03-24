package registry

import "github.com/erickivel/oksg/pkg/core/metrics"

type MetricRegistry interface {
	Register(metric metrics.CanonicalMetric) error
	Get(name string) (metrics.CanonicalMetric, bool)
	List() []metrics.CanonicalMetric
}
