package istio

import (
	"context"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/semconv"
	"github.com/erickivel/oksg/pkg/sources/contract"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	"strings"
)

func Normalize(ctx context.Context, resolver contract.EntityResolver, plan *contract.Plan) error {
	if !strings.HasPrefix(plan.RawMetricName, "istio_") && !strings.HasPrefix(plan.MetricName, "servicegraph.") {
		return nil
	}
	contract.PreserveTraceLabels(plan.Attrs, plan.Labels)
	contract.PreserveLabels(plan.Attrs, plan.Labels, semconv.AttrClientNamespace, semconv.AttrServerNamespace)
	preserveMesh(plan.Attrs, plan.Labels)
	plan.Skip = !contract.ApplyServiceGraphEdgeLabels(ctx, resolver, plan)
	if plan.Skip {
		return nil
	}
	if plan.RawMetricName == "istio_request_duration_milliseconds" {
		plan.ConversionFactor = 1e6
	}
	if strings.HasPrefix(contract.FirstLabelValue(plan.Attrs, semconv.AttrHTTPStatusCode, semconv.AttrHTTPStatusCodeProm), "5") && plan.RawMetricName == "istio_requests_total" {
		mapping := metrics.OTLPSemconvMetrics["servicegraph.request.error.count"]
		plan.Derived = append(plan.Derived, contract.DerivedMetric{MetricName: "servicegraph.request.error.count", ConversionFactor: mapping.ConversionFactor, Unit: mapping.Unit})
	}
	return nil
}

// CallsEdgeMetadata recognizes Istio's servicegraph-shaped metrics and keeps
// only its bounded mesh dimensions for the generic calls edge.
func CallsEdgeMetadata(rawMetricName, canonicalMetricName string, attrs []*commonpb.KeyValue) (map[string]string, bool) {
	if !strings.HasPrefix(rawMetricName, "istio_") || !strings.HasPrefix(canonicalMetricName, "servicegraph.") {
		return nil, false
	}
	metadata := make(map[string]string)
	for _, key := range meshAttributeKeys {
		if value := MeshAttributeValue(attrs, key); value != "" {
			metadata[key] = value
		}
	}
	return metadata, true
}

var meshAttributeKeys = []string{
	semconv.AttrNetworkProtocolName,
	semconv.AttrHTTPStatusCode,
	semconv.AttrGRPCStatusCode,
	semconv.AttrMeshPolicy,
	semconv.AttrMeshSourcePrincipal,
	semconv.AttrMeshDestinationPrincipal,
	semconv.AttrMeshResponseFlags,
	semconv.AttrMeshID,
}

func preserveMesh(attrs []*commonpb.KeyValue, labels map[string]string) {
	for _, key := range meshAttributeKeys {
		if value := MeshAttributeValue(attrs, key); value != "" {
			labels[key] = value
		}
	}
}
func MeshAttributeValue(attrs []*commonpb.KeyValue, key string) string {
	switch key {
	case semconv.AttrNetworkProtocolName:
		return contract.FirstLabelValue(attrs, key, semconv.AttrNetworkProtocolNameProm)
	case semconv.AttrHTTPStatusCode:
		return contract.FirstLabelValue(attrs, key, semconv.AttrHTTPStatusCodeProm)
	case semconv.AttrGRPCStatusCode:
		return contract.FirstLabelValue(attrs, key, semconv.AttrGRPCStatusCodeProm)
	case semconv.AttrMeshPolicy:
		return contract.FirstLabelValue(attrs, key, semconv.AttrMeshPolicyProm)
	case semconv.AttrMeshSourcePrincipal:
		return contract.FirstLabelValue(attrs, key, semconv.AttrMeshSourcePrincipalProm)
	case semconv.AttrMeshDestinationPrincipal:
		return contract.FirstLabelValue(attrs, key, semconv.AttrMeshDestinationPrincipalProm)
	case semconv.AttrMeshResponseFlags:
		return contract.FirstLabelValue(attrs, key, semconv.AttrMeshResponseFlagsProm)
	case semconv.AttrMeshID:
		return contract.FirstLabelValue(attrs, key, semconv.AttrMeshIDProm)
	}
	return contract.LabelValue(attrs, key)
}
