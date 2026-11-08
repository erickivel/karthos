package servicegraph

import (
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/sources/contract"
)

func QueryViews() []contract.QueryView {
	return []contract.QueryView{{SourceID: metrics.SourceTracesServicegraph, MetricName: "servicegraph.request.error.count", InputMetric: "servicegraph.request.error.count", Branches: []contract.QueryBranch{{}}, Derivation: metrics.Derivation{ID: "servicegraph.connector-failures", Version: "1", InputMetric: "servicegraph.request.error.count", Interpretation: "Connector-native failed-request counter; not equivalent to the Istio status policy"}}}
}
