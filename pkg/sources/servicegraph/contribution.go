package servicegraph

import (
	"fmt"

	collectorspec "github.com/erickivel/karthos/pkg/collector/spec"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/discovery/profile"
)

// DefaultStoreMaxItems bounds unmatched span-pair state in the gateway.
const DefaultStoreMaxItems = 10000

// Options is the servicegraph source's operator-controlled collection policy.
type Options struct{ StoreMaxItems int }

func DefaultOptions() Options { return Options{StoreMaxItems: DefaultStoreMaxItems} }

func (o Options) Validate() error {
	if o.StoreMaxItems <= 0 {
		return fmt.Errorf("servicegraph store max items must be a positive integer")
	}
	return nil
}

// Contribute preserves the default source constructor for existing callers.
func Contribute(plan *collectorspec.CollectionPlan, capability profile.SourceCapability, selected bool) {
	DefaultOptions().Contribute(plan, capability, selected)
}

func (o Options) Contribute(plan *collectorspec.CollectionPlan, capability profile.SourceCapability, selected bool) {
	if capability.Evidence.OTLP != nil {
		plan.Agent.OTLP.Traces = plan.Agent.OTLP.Traces || capability.Evidence.OTLP.TracesEnabled
		plan.Gateway.OTLP.Traces = plan.Gateway.OTLP.Traces || capability.Evidence.OTLP.TracesEnabled
	}
	if !selected {
		return
	}
	plan.Gateway.ServiceGraph = &collectorspec.ServiceGraph{SourceID: metrics.SourceTracesServicegraph, MetricsFlushInterval: "15s", LatencyHistogramBuckets: []string{"100ms", "250ms", "500ms", "1s", "5s"}, Dimensions: []string{"http.method", "k8s.namespace.name"}, StoreTTL: "30s", StoreMaxItems: o.StoreMaxItems}
	plan.Gateway.SourceStamps = append(plan.Gateway.SourceStamps, collectorspec.SourceStamp{SourceID: metrics.SourceTracesServicegraph, MetricPatterns: []string{`^traces_service_graph_`, `^traces\.service\.graph\.`}, OnlyIfUnset: true})
}
