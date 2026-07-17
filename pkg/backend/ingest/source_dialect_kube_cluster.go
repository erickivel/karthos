package ingest

import (
	"context"
	"strings"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/semconv"
)

func kubeClusterSourceDialect() sourceDialect {
	return sourceDialect{
		sourceID: metrics.SourceKubeCluster,
		matches: func(_ string, canonicalName string) bool {
			return canonicalName == "k8s.node.condition" || isResourceQuotaMetric(canonicalName)
		},
		apply: applyKubeClusterDialect,
	}
}

func applyKubeClusterDialect(_ context.Context, _ EntityResolver, plan *normalizationPlan) error {
	preserveLabels(plan.attrs, plan.labels,
		semconv.AttrResource,
		semconv.ResourceK8sPodName,
		semconv.ResourceK8sNamespaceName,
	)

	if plan.metricName == "k8s.node.condition" {
		switch kvString(plan.attrs, semconv.AttrCondition) {
		case "Ready":
			plan.metricName = "k8s.node.condition.ready"
		case "MemoryPressure":
			plan.metricName = "k8s.node.condition.memory_pressure"
		case "DiskPressure":
			plan.metricName = "k8s.node.condition.disk_pressure"
		default:
			plan.skip = true
		}
	}
	if isResourceQuotaMetric(plan.metricName) {
		plan.unit = resourceQuotaUnit(kvString(plan.attrs, semconv.AttrResource))
	}
	return nil
}

func isResourceQuotaMetric(metricName string) bool {
	return metricName == "k8s.resource_quota.hard_limit" || metricName == "k8s.resource_quota.used"
}

func resourceQuotaUnit(resource string) metrics.Unit {
	switch strings.ToLower(resource) {
	case "cpu", "requests.cpu", "limits.cpu":
		return metrics.UnitMillicores
	case "memory", "requests.memory", "limits.memory",
		"ephemeral-storage", "requests.ephemeral-storage", "limits.ephemeral-storage",
		"storage", "requests.storage":
		return metrics.UnitBytes
	case "pods", "services", "services.loadbalancers", "services.nodeports",
		"configmaps", "secrets", "persistentvolumeclaims", "replicationcontrollers",
		"resourcequotas":
		return metrics.UnitCount
	}
	if strings.HasPrefix(resource, "count/") {
		return metrics.UnitCount
	}
	if strings.HasPrefix(resource, "requests.") && strings.HasSuffix(resource, "/storage") {
		return metrics.UnitBytes
	}
	if strings.HasSuffix(resource, "/requests.storage") || strings.Contains(resource, "hugepages-") {
		return metrics.UnitBytes
	}
	return metrics.UnitResource
}
