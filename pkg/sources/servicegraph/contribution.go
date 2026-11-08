package servicegraph

import (
	collectorspec "github.com/erickivel/karthos/pkg/collector/spec"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/discovery/profile"
)

func Contribute(plan *collectorspec.CollectionPlan, capability profile.SourceCapability, selected bool) {
	if capability.Evidence.OTLP != nil {
		plan.Agent.OTLP.Traces = plan.Agent.OTLP.Traces || capability.Evidence.OTLP.TracesEnabled
		plan.Gateway.OTLP.Traces = plan.Gateway.OTLP.Traces || capability.Evidence.OTLP.TracesEnabled
	}
	if !selected {
		return
	}
	plan.Gateway.ServiceGraph = &collectorspec.ServiceGraph{SourceID: metrics.SourceTracesServicegraph, MetricsFlushInterval: "15s", LatencyHistogramBuckets: []string{"100ms", "250ms", "500ms", "1s", "5s"}, Dimensions: []string{"http.method", "k8s.namespace.name"}, StoreTTL: "30s", StoreMaxItems: 1000}
	plan.Gateway.SourceStamps = append(plan.Gateway.SourceStamps, collectorspec.SourceStamp{SourceID: metrics.SourceTracesServicegraph, MetricPatterns: []string{`^traces_service_graph_`, `^traces\.service\.graph\.`}, OnlyIfUnset: true})
}
