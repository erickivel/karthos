package prometheusapp

import "github.com/erickivel/karthos/pkg/sources/contract"

func Parse(rawMetricName string, _ map[string]string) (contract.ParsedMetric, bool) {
	canonical, ok := rawMetricNames[rawMetricName]
	if !ok {
		return contract.ParsedMetric{}, false
	}
	return contract.ParsedMetric{CanonicalName: canonical, ConversionFactor: 1}, true
}
