package kubecluster

import (
	"context"
	"github.com/erickivel/karthos/pkg/core/semconv"
	"github.com/erickivel/karthos/pkg/sources/contract"
)

func Normalize(_ context.Context, _ contract.EntityResolver, plan *contract.Plan) error {
	contract.PreserveLabels(plan.Attrs, plan.Labels, semconv.AttrResource, semconv.ResourceK8sPodName, semconv.ResourceK8sNamespaceName)
	return nil
}
