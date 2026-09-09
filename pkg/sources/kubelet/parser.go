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
	factor := 1.0
	switch canonical {
	case "k8s.pod.cpu.usage", "k8s.node.cpu.usage", "k8s.container.cpu.usage":
		factor = 1000.0
	}
	return contract.ParsedMetric{CanonicalName: canonical, ConversionFactor: factor}, true
}

var nativeMetricNames = map[string]struct{}{
	"k8s.pod.cpu.usage":           {},
	"k8s.pod.memory.working_set":  {},
	"k8s.node.cpu.usage":          {},
	"k8s.node.memory.working_set": {},
	"k8s.container.cpu.usage":     {},
	"k8s.container.memory.usage":  {},
	"k8s.volume.available":        {},
	"k8s.volume.capacity":         {},
	"k8s.volume.inodes.free":      {},
	"k8s.volume.inodes.used":      {},
	"k8s.volume.inodes":           {},
}
