package ingest

import (
	"context"
	"strings"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/semconv"
)

func servicegraphSourceDialect() sourceDialect {
	return sourceDialect{
		sourceID: metrics.SourceTracesServicegraph,
		matches: func(rawMetricName, _ string) bool {
			return strings.HasPrefix(rawMetricName, "traces_service_graph_") ||
				strings.HasPrefix(rawMetricName, "traces.service.graph.")
		},
		apply: applyServicegraphDialect,
	}
}

func applyServicegraphDialect(ctx context.Context, resolver EntityResolver, plan *normalizationPlan) error {
	if !strings.HasPrefix(plan.rawMetricName, "traces_service_graph_") &&
		!strings.HasPrefix(plan.rawMetricName, "traces.service.graph.") &&
		!strings.HasPrefix(plan.metricName, "servicegraph.") {
		return nil
	}
	preserveTraceLabels(plan.attrs, plan.labels)
	preserveLabels(plan.attrs, plan.labels,
		"failed",
		"connection_type",
		"virtual_node",
		"client_http.method:client_http_method",
		"server_http.method:server_http_method",
		semconv.AttrClientNamespace,
		semconv.AttrServerNamespace,
	)
	plan.skip = !applyServiceGraphEdgeLabels(ctx, resolver, plan)
	return nil
}
