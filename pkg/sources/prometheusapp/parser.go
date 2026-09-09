package prometheusapp

import "github.com/erickivel/karthos/pkg/sources/contract"

func Parse(rawMetricName string, _ map[string]string) (contract.ParsedMetric, bool) {
	canonical, ok := rawMetricNames[rawMetricName]
	if !ok {
		return contract.ParsedMetric{}, false
	}
	factor := 1.0
	switch canonical {
	case "k8s.container.cpu.usage":
		factor = 1000.0
	case "http.server.request.duration":
		factor = 1e9
	}
	return contract.ParsedMetric{CanonicalName: canonical, ConversionFactor: factor}, true
}
