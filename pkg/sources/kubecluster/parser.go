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
	factor := 1.0
	switch canonical {
	case "k8s.container.cpu.request", "k8s.container.cpu.limit":
		factor = 1000.0
	}
	parsed := contract.ParsedMetric{CanonicalName: canonical, ConversionFactor: factor}
	return parsed, true
}

var nativeMetricNames = map[string]struct{}{
	"k8s.container.ready":    {},
	"k8s.container.restarts": {},
	"k8s.pod.phase":          {},
	"k8s.pod.status_reason":  {},
	"k8s.node.condition":     {},
}
