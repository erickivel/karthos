package ingest

import (
	"context"
	"strings"

	commonpb "go.opentelemetry.io/proto/otlp/common/v1"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/semconv"
)

func istioSourceDialect() sourceDialect {
	return sourceDialect{
		sourceID: metrics.SourceMeshIstio,
		matches: func(rawMetricName, _ string) bool {
			return strings.HasPrefix(rawMetricName, "istio_")
		},
		apply: applyIstioDialect,
	}
}

func applyIstioDialect(ctx context.Context, resolver EntityResolver, plan *normalizationPlan) error {
	if !strings.HasPrefix(plan.rawMetricName, "istio_") && !strings.HasPrefix(plan.metricName, "servicegraph.") {
		return nil
	}
	preserveTraceLabels(plan.attrs, plan.labels)
	preserveLabels(plan.attrs, plan.labels, semconv.AttrClientNamespace, semconv.AttrServerNamespace)
	applyMeshMetadataLabels(plan.attrs, plan.labels)
	plan.skip = !applyServiceGraphEdgeLabels(ctx, resolver, plan)
	if plan.skip {
		return nil
	}

	if plan.rawMetricName == "istio_request_duration_milliseconds" {
		plan.conversionFactor = 1e6
	}
	if shouldEmitIstioErrorCount(plan.rawMetricName, plan.attrs) {
		errorMapping := metrics.OTLPSemconvMetrics["servicegraph.request.error.count"]
		plan.derived = append(plan.derived, derivedMetricPlan{
			metricName:       "servicegraph.request.error.count",
			conversionFactor: errorMapping.ConversionFactor,
			unit:             errorMapping.Unit,
		})
	}
	return nil
}

func applyMeshMetadataLabels(attrs []*commonpb.KeyValue, labels map[string]string) {
	for _, key := range []string{
		semconv.AttrNetworkProtocolName,
		semconv.AttrHTTPStatusCode,
		semconv.AttrGRPCStatusCode,
		semconv.AttrMeshPolicy,
		semconv.AttrMeshSourcePrincipal,
		semconv.AttrMeshDestinationPrincipal,
		semconv.AttrMeshResponseFlags,
		semconv.AttrMeshID,
	} {
		if value := meshAttrValue(attrs, key); value != "" {
			labels[key] = value
		}
	}
}

func shouldEmitIstioErrorCount(metricName string, attrs []*commonpb.KeyValue) bool {
	if metricName != "istio_requests_total" {
		return false
	}
	status := firstKVString(attrs, semconv.AttrHTTPStatusCode, semconv.AttrHTTPStatusCodeProm)
	return strings.HasPrefix(status, "5")
}

func meshAttrValue(attrs []*commonpb.KeyValue, canonical string) string {
	switch canonical {
	case semconv.AttrNetworkProtocolName:
		return firstKVString(attrs, canonical, semconv.AttrNetworkProtocolNameProm)
	case semconv.AttrHTTPStatusCode:
		return firstKVString(attrs, canonical, semconv.AttrHTTPStatusCodeProm)
	case semconv.AttrGRPCStatusCode:
		return firstKVString(attrs, canonical, semconv.AttrGRPCStatusCodeProm)
	case semconv.AttrMeshPolicy:
		return firstKVString(attrs, canonical, semconv.AttrMeshPolicyProm)
	case semconv.AttrMeshSourcePrincipal:
		return firstKVString(attrs, canonical, semconv.AttrMeshSourcePrincipalProm)
	case semconv.AttrMeshDestinationPrincipal:
		return firstKVString(attrs, canonical, semconv.AttrMeshDestinationPrincipalProm)
	case semconv.AttrMeshResponseFlags:
		return firstKVString(attrs, canonical, semconv.AttrMeshResponseFlagsProm)
	case semconv.AttrMeshID:
		return firstKVString(attrs, canonical, semconv.AttrMeshIDProm)
	default:
		return kvString(attrs, canonical)
	}
}
