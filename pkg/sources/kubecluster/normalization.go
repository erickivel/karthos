package kubecluster

import (
	"context"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/semconv"
	"github.com/erickivel/oksg/pkg/sources/contract"
	"strings"
)

func Normalize(_ context.Context, _ contract.EntityResolver, plan *contract.Plan) error {
	contract.PreserveLabels(plan.Attrs, plan.Labels, semconv.AttrResource, semconv.ResourceK8sPodName, semconv.ResourceK8sNamespaceName)
	if plan.MetricName == "k8s.node.condition" {
		switch contract.LabelValue(plan.Attrs, semconv.AttrCondition) {
		case "Ready":
			plan.MetricName = "k8s.node.condition.ready"
		case "MemoryPressure":
			plan.MetricName = "k8s.node.condition.memory_pressure"
		case "DiskPressure":
			plan.MetricName = "k8s.node.condition.disk_pressure"
		default:
			plan.Skip = true
		}
	}
	if plan.MetricName == "k8s.resource_quota.hard_limit" || plan.MetricName == "k8s.resource_quota.used" {
		plan.Unit = quotaUnit(contract.LabelValue(plan.Attrs, semconv.AttrResource))
	}
	return nil
}
func quotaUnit(resource string) metrics.Unit {
	switch strings.ToLower(resource) {
	case "cpu", "requests.cpu", "limits.cpu":
		return metrics.UnitMillicores
	case "memory", "requests.memory", "limits.memory", "ephemeral-storage", "requests.ephemeral-storage", "limits.ephemeral-storage", "storage", "requests.storage":
		return metrics.UnitBytes
	case "pods", "services", "services.loadbalancers", "services.nodeports", "configmaps", "secrets", "persistentvolumeclaims", "replicationcontrollers", "resourcequotas":
		return metrics.UnitCount
	}
	if strings.HasPrefix(resource, "count/") {
		return metrics.UnitCount
	}
	if strings.HasPrefix(resource, "requests.") && strings.HasSuffix(resource, "/storage") || strings.HasSuffix(resource, "/requests.storage") || strings.Contains(resource, "hugepages-") {
		return metrics.UnitBytes
	}
	return metrics.UnitResource
}
