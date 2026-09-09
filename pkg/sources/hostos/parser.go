package hostos

import (
	"github.com/erickivel/karthos/pkg/core/semconv"
	"github.com/erickivel/karthos/pkg/sources/contract"
)

func Parse(rawMetricName string, attributes map[string]string) (contract.ParsedMetric, bool) {
	if _, supported := nativeMetricNames[rawMetricName]; !supported {
		return contract.ParsedMetric{}, false
	}
	canonical := rawMetricName
	switch rawMetricName {
	case "system.disk.io":
		switch attributes[semconv.AttrDirection] {
		case "read":
			canonical = "system.disk.io.read"
		case "write":
			canonical = "system.disk.io.write"
		default:
			return contract.ParsedMetric{}, false
		}
	case "system.network.io":
		switch attributes[semconv.AttrDirection] {
		case "receive":
			canonical = "system.network.io.receive"
		case "transmit":
			canonical = "system.network.io.transmit"
		default:
			return contract.ParsedMetric{}, false
		}
	}
	factor := 1.0
	if canonical == "system.cpu.utilization" || canonical == "system.filesystem.utilization" {
		factor = 100.0
	}
	return contract.ParsedMetric{CanonicalName: canonical, ConversionFactor: factor}, true
}

var nativeMetricNames = map[string]struct{}{
	"system.cpu.utilization":        {},
	"system.memory.usage":           {},
	"system.disk.io":                {},
	"system.disk.operations":        {},
	"system.filesystem.usage":       {},
	"system.filesystem.utilization": {},
	"system.network.io":             {},
	"system.network.errors":         {},
}
