package istio

import "github.com/erickivel/oksg/pkg/sources/contract"

func Parse(rawMetricName string, _ map[string]string) (contract.ParsedMetric, bool) {
	canonical, ok := rawMetricNames[rawMetricName]
	if !ok {
		return contract.ParsedMetric{}, false
	}
	factor := 1.0
	if rawMetricName == "istio_request_duration_milliseconds" {
		factor = 1e6
	}
	return contract.ParsedMetric{CanonicalName: canonical, ConversionFactor: factor}, true
}
