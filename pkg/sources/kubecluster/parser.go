package kubecluster

import (
	"github.com/erickivel/karthos/pkg/core/semconv"
	"github.com/erickivel/karthos/pkg/sources/contract"
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
	parsed := contract.ParsedMetric{CanonicalName: canonical, ConversionFactor: 1}
	return parsed, true
}

var nativeMetricNames = map[string]struct{}{
	"k8s.container.ready": {},
	"k8s.node.condition":  {},
}
