package prometheusapp

import (
	collectorspec "github.com/erickivel/oksg/pkg/collector/spec"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

func Contribute(plan *collectorspec.CollectionPlan, capability profile.SourceCapability, selected bool) {
	if !selected || len(capability.Evidence.PrometheusTargets) == 0 {
		return
	}
	targets := append([]profile.PrometheusTarget(nil), capability.Evidence.PrometheusTargets...)
	plan.Gateway.Prometheus = &collectorspec.Prometheus{SourceID: metrics.SourcePrometheusApp, Targets: targets}
}
