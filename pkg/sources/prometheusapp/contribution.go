package prometheusapp

import (
	collectorspec "github.com/erickivel/karthos/pkg/collector/spec"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/discovery/profile"
)

func Contribute(plan *collectorspec.CollectionPlan, capability profile.SourceCapability, selected bool) {
	if !selected || len(capability.Evidence.PrometheusTargets) == 0 {
		return
	}
	targets := append([]profile.PrometheusTarget(nil), capability.Evidence.PrometheusTargets...)
	plan.Gateway.Prometheus = &collectorspec.Prometheus{SourceID: metrics.SourcePrometheusApp, Targets: targets}
}
