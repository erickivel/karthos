package ingest

import (
	"context"
	"strings"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/semconv"
)

func spanmetricsSourceDialect() sourceDialect {
	return sourceDialect{
		sourceID: metrics.SourceTracesSpanmetrics,
		matches: func(rawMetricName, _ string) bool {
			return strings.HasPrefix(rawMetricName, "traces.span.metrics.")
		},
		apply: applySpanmetricsDialect,
	}
}

func applySpanmetricsDialect(ctx context.Context, resolver EntityResolver, plan *normalizationPlan) error {
	if !strings.HasPrefix(plan.rawMetricName, "traces.span.metrics.") {
		return nil
	}
	preserveTraceLabels(plan.attrs, plan.labels)
	serviceName := plan.resource[semconv.ResourceServiceName]
	if serviceName == "" {
		plan.skip = true
		return nil
	}
	if serviceID, ok := resolveServiceMetricEntityID(
		ctx,
		resolver,
		plan.resource,
		serviceName,
		plan.resource[semconv.ResourceK8sNamespaceName],
	); ok {
		plan.entityID = serviceID
		return nil
	}
	plan.skip = true
	return nil
}

func preserveTraceLabels(attrs []*commonpb.KeyValue, labels map[string]string) {
	preserveLabels(attrs, labels,
		"http.method:http_method",
		"http_method",
		semconv.AttrHTTPStatusCode+":http_status_code",
		semconv.AttrHTTPStatusCodeProm+":http_status_code",
		"http.status_code:http_status_code",
		"status_code",
		"status.code:status_code",
		"span.name:span_name",
		"span_name",
		"span.kind:span_kind",
		"span_kind",
	)
}
