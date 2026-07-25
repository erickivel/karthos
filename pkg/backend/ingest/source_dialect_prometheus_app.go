package ingest

import (
	"context"
	"strings"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/semconv"
)

func prometheusAppSourceDialect() sourceDialect {
	return sourceDialect{
		sourceID: metrics.SourcePrometheusApp,
		matches: func(rawMetricName, canonicalName string) bool {
			return (rawMetricName == "http_requests_total" || rawMetricName == "http_request_duration_seconds") &&
				strings.HasPrefix(canonicalName, "http.server.")
		},
		apply: applyPrometheusAppDialect,
	}
}

func applyPrometheusAppDialect(ctx context.Context, resolver EntityResolver, plan *normalizationPlan) error {
	preserveLabels(plan.attrs, plan.labels,
		"namespace",
		"service",
		"metrics_path",
		"service_port",
		"method",
		"status",
		"http_method",
		"http.route:http_route",
		"http_route",
		"http.status_code:http_status_code",
		semconv.AttrHTTPStatusCode+":http_status_code",
		semconv.AttrHTTPStatusCodeProm+":http_status_code",
	)

	serviceName := firstKVString(plan.attrs, "service", semconv.ResourceServiceName)
	namespace := firstKVString(plan.attrs, "namespace", semconv.ResourceK8sNamespaceName)
	if serviceName == "" || namespace == "" {
		return nil
	}
	if serviceID, ok := resolveServiceMetricEntityID(ctx, resolver, plan.resource, serviceName, namespace); ok {
		plan.entityID = serviceID
	}
	return nil
}
