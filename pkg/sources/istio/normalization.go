package istio

import (
	"context"
	"github.com/erickivel/karthos/pkg/core/semconv"
	"github.com/erickivel/karthos/pkg/sources/contract"
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
	return nil
}

// CallsEdgeMetadata recognizes request observations and new TCP connections.
func CallsEdgeMetadata(rawMetricName, canonicalMetricName string, attrs []*commonpb.KeyValue) (map[string]string, bool) {
	if rawMetricNames[rawMetricName] != canonicalMetricName {
		return nil, false
	}
	switch canonicalMetricName {
	case "servicegraph.request.count", "servicegraph.request.duration", "servicegraph.tcp.connection.open.count":
	default:
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
