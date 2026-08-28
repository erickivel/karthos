package kubecluster

import (
	"strings"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/semconv"
	"github.com/erickivel/oksg/pkg/sources/contract"
)

func Parse(rawMetricName string, attributes map[string]string) (contract.ParsedMetric, bool) {
	canonical, ok := rawMetricNames[rawMetricName]
	if !ok {
		if _, supported := nativeMetricNames[rawMetricName]; !supported {
			return contract.ParsedMetric{}, false
		}
		canonical = rawMetricName
	}
	if canonical == "k8s.node.condition" {
		switch attributes[semconv.AttrCondition] {
		case "Ready":
			canonical = "k8s.node.condition.ready"
		case "MemoryPressure":
			canonical = "k8s.node.condition.memory_pressure"
		case "DiskPressure":
			canonical = "k8s.node.condition.disk_pressure"
		default:
			return contract.ParsedMetric{}, false
		}
	}
	factor := 1.0
	switch canonical {
	case "k8s.container.cpu.request", "k8s.container.cpu.limit":
		factor = 1000.0
	}
	parsed := contract.ParsedMetric{CanonicalName: canonical, ConversionFactor: factor}
	if canonical == "k8s.resource_quota.hard_limit" || canonical == "k8s.resource_quota.used" {
		parsed.UnitOverride = quotaUnit(attributes[semconv.AttrResource])
	}
	return parsed, true
}

var nativeMetricNames = map[string]struct{}{
	"k8s.container.ready":           {},
	"k8s.container.restarts":        {},
	"k8s.pod.phase":                 {},
	"k8s.pod.status_reason":         {},
	"k8s.node.condition":            {},
	"k8s.resource_quota.hard_limit": {},
	"k8s.resource_quota.used":       {},
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
