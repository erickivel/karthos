package servicegraph

import (
	"context"
	"github.com/erickivel/oksg/pkg/core/semconv"
	"github.com/erickivel/oksg/pkg/sources/contract"
	"strings"
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
