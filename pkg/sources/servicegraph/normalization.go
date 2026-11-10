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
	for _, side := range []struct{ prefix, target string }{{"client", semconv.AttrClientNamespace}, {"server", semconv.AttrServerNamespace}} {
		if contract.LabelValue(plan.Attrs, side.target) != "" {
			continue
		}
		value := contract.FirstLabelValue(plan.Attrs, side.prefix+"_k8s.namespace.name", side.prefix+"_k8s_namespace_name")
		if value != "" {
			plan.Attrs = append(plan.Attrs, &commonpb.KeyValue{Key: side.target, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: value}}})
		}
	}
	contract.PreserveTraceLabels(plan.Attrs, plan.Labels)
	contract.PreserveLabels(plan.Attrs, plan.Labels, "failed", "connection_type", "virtual_node", "client_http.method:client_http_method", "server_http.method:server_http_method", semconv.AttrClientNamespace, semconv.AttrServerNamespace)
	plan.Skip = !contract.ApplyServiceGraphEdgeLabels(ctx, resolver, plan)
	return nil
}

// CallsEdgeMetadata recognizes metrics emitted by the derived servicegraph
// source. Its calls edges need no source-specific metadata.
func CallsEdgeMetadata(rawMetricName, canonicalMetricName string, _ []*commonpb.KeyValue) (map[string]string, bool) {
	if rawMetricNames[rawMetricName] != canonicalMetricName {
		return nil, false
	}
	switch canonicalMetricName {
	case "servicegraph.request.count", "servicegraph.request.error.count", "servicegraph.request.duration":
		return nil, true
	default:
		return nil, false
	}
}

// CallsNamespaces reads the side-specific dimensions emitted by the deployed
// servicegraph connector. Both sides must be present; no namespace is guessed.
func CallsNamespaces(attrs []*commonpb.KeyValue) (string, string) {
	return contract.FirstLabelValue(attrs, "client_k8s.namespace.name", "client_k8s_namespace_name"),
		contract.FirstLabelValue(attrs, "server_k8s.namespace.name", "server_k8s_namespace_name")
}
