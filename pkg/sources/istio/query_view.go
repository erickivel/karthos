package istio

import (
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/core/semconv"
	"github.com/erickivel/karthos/pkg/sources/contract"
)

func QueryViews() []contract.QueryView {
	m := func(name, op, value string) contract.LabelMatcher {
		return contract.LabelMatcher{Name: name, Operator: op, Value: value}
	}
	http := []contract.LabelMatcher{m(semconv.AttrNetworkProtocolName, "!=", "grpc"), m(semconv.AttrGRPCStatusCode, "=", "")}
	return []contract.QueryView{{SourceID: metrics.SourceMeshIstio, MetricName: "servicegraph.request.error.count", InputMetric: "servicegraph.request.count",
		Derivation: metrics.Derivation{ID: "istio.request-errors", Version: "1", InputMetric: "servicegraph.request.count", Interpretation: "HTTP 500–599 for non-gRPC; nonzero gRPC status for RPCs; missing RPC status unknown"},
		Branches: []contract.QueryBranch{
			{Matchers: []contract.LabelMatcher{m(semconv.AttrGRPCStatusCode, "=~", "[1-9][0-9]*")}},
			{Matchers: []contract.LabelMatcher{m(semconv.AttrGRPCStatusCode, "=", "0")}, Zero: true},
			{Matchers: append(append([]contract.LabelMatcher{}, http...), m(semconv.AttrHTTPStatusCode, "=~", "5[0-9][0-9]"))},
			{Matchers: append(append([]contract.LabelMatcher{}, http...), m(semconv.AttrHTTPStatusCode, "=~", "[1-4][0-9][0-9]")), Zero: true},
		}}}
}
