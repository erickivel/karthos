package kubelet

import "github.com/erickivel/karthos/pkg/sources/contract"

func Parse(rawMetricName string, _ map[string]string) (contract.ParsedMetric, bool) {
	canonical, ok := rawMetricNames[rawMetricName]
	if !ok {
		if _, supported := nativeMetricNames[rawMetricName]; !supported {
			return contract.ParsedMetric{}, false
		}
		canonical = rawMetricName
	}
	return contract.ParsedMetric{CanonicalName: canonical, ConversionFactor: 1}, true
}

var nativeMetricNames = map[string]struct{}{
	"k8s.pod.cpu.usage":           {},
	"k8s.pod.memory.working_set":  {},
	"k8s.node.cpu.usage":          {},
	"k8s.node.memory.working_set": {},
	"container.cpu.usage":         {},
	"container.memory.usage":      {},
}
