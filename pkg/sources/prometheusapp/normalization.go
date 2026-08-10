package prometheusapp

import (
	"context"
	"github.com/erickivel/oksg/pkg/core/semconv"
	"github.com/erickivel/oksg/pkg/sources/contract"
)

func Normalize(ctx context.Context, resolver contract.EntityResolver, plan *contract.Plan) error {
	contract.PreserveLabels(plan.Attrs, plan.Labels, "namespace", "service", "metrics_path", "service_port", "method", "status", "http_method", "http.route:http_route", "http_route", "http.status_code:http_status_code", semconv.AttrHTTPStatusCode+":http_status_code", semconv.AttrHTTPStatusCodeProm+":http_status_code")
	service := contract.FirstLabelValue(plan.Attrs, "service", semconv.ResourceServiceName)
	namespace := contract.FirstLabelValue(plan.Attrs, "namespace", semconv.ResourceK8sNamespaceName)
	if id, ok := contract.ResolveServiceMetricEntity(ctx, resolver, plan.Resource, service, namespace); ok {
		plan.EntityID = id
	}
	return nil
}
