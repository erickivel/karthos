package victoriametrics

import (
	"github.com/erickivel/karthos/pkg/backend/query"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/stretchr/testify/require"
	"testing"
	"time"
)

func TestErrorViewUsesRequestCountersBeforeRollupAndIgnoresLegacy(t *testing.T) {
	for _, mode := range []metrics.AggregationMode{metrics.AggregationLatest, metrics.AggregationIncrease, metrics.AggregationWindowRate, metrics.AggregationActiveRate} {
		q := query.MetricQuery{MetricName: "servicegraph.request.error.count", Source: "mesh.istio", Aggregation: mode, Window: time.Minute, MaxSampleAge: time.Minute}
		text := RenderQuery(q)
		require.Contains(t, text, "servicegraph_request_count")
		require.NotContains(t, text, "servicegraph_request_error_count")
		require.Contains(t, text, `source="mesh.istio"`)
		require.Contains(t, text, `rpc\.grpc\.status_code`)
		require.Contains(t, text, `network\.protocol\.name`)
		require.Contains(t, text, `http\.response\.status_code`)
		require.Contains(t, text, " * 0")
	}
}
func TestTraceNativeQueryKeepsInputAndDifferentProvenance(t *testing.T) {
	text := RenderQuery(query.MetricQuery{MetricName: "servicegraph.request.error.count", Source: "traces.servicegraph", Aggregation: metrics.AggregationLatest})
	require.Contains(t, text, `servicegraph_request_error_count{source="traces.servicegraph"}`)
	require.NotContains(t, text, "servicegraph_request_count{")
}
func TestSourceSelectorAppliesToOrdinaryQueries(t *testing.T) {
	text := RenderQuery(query.MetricQuery{MetricName: "k8s.container.memory.usage", Source: "kubelet"})
	require.Contains(t, text, `source="kubelet"`)
}
