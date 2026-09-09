package servicegraph

import (
	"context"
	"strings"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"

	"github.com/erickivel/karthos/pkg/core/semconv"
	"github.com/erickivel/karthos/pkg/sources/contract"
)

func Normalize(ctx context.Context, resolver contract.EntityResolver, plan *contract.Plan) error {
	if !strings.HasPrefix(plan.RawMetricName, "traces_service_graph_") && !strings.HasPrefix(plan.RawMetricName, "traces.service.graph.") && !strings.HasPrefix(plan.MetricName, "servicegraph.") {
		return nil
	}
	contract.PreserveTraceLabels(plan.Attrs, plan.Labels)
	contract.PreserveLabels(plan.Attrs, plan.Labels, "failed", "connection_type", "virtual_node", "client_http.method:client_http_method", "server_http.method:server_http_method", semconv.AttrClientNamespace, semconv.AttrServerNamespace)
	plan.Skip = !contract.ApplyServiceGraphEdgeLabels(ctx, resolver, plan)
	return nil
}

// CallsEdgeMetadata recognizes metrics emitted by the derived servicegraph
// source. Its calls edges need no source-specific metadata.
func CallsEdgeMetadata(rawMetricName, canonicalMetricName string, _ []*commonpb.KeyValue) (map[string]string, bool) {
	if !strings.HasPrefix(rawMetricName, "traces_service_graph_") && !strings.HasPrefix(rawMetricName, "traces.service.graph.") && !strings.HasPrefix(canonicalMetricName, "servicegraph.") {
		return nil, false
	}
	return nil, true
}
