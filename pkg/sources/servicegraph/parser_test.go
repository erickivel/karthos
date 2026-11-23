package servicegraph

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTranslatesEverySupportedDerivedServiceGraphMetric(t *testing.T) {
	for raw, expected := range map[string]struct {
		canonical string
		factor    float64
	}{
		"traces_service_graph_request_total":        {"servicegraph.request.count", 1},
		"traces_service_graph_request_server":       {"servicegraph.request.duration", 1},
		"traces_service_graph_request_failed_total": {"servicegraph.request.error.count", 1},
		"traces.service.graph.request.total":        {"servicegraph.request.count", 1},
		"traces.service.graph.request.server":       {"servicegraph.request.duration", 1},
		"traces.service.graph.request_failed_total": {"servicegraph.request.error.count", 1},
	} {
		parsed, ok := Parse(raw, nil)
		require.True(t, ok)
		require.Equal(t, expected.canonical, parsed.CanonicalName)
		require.Equal(t, expected.factor, parsed.ConversionFactor)
	}
}
