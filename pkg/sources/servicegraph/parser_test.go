package servicegraph

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestParseTranslatesDerivedServiceGraphMetrics(t *testing.T) {
	for raw, canonical := range map[string]string{
		"traces_service_graph_request_total":  "servicegraph.request.count",
		"traces_service_graph_request_server": "servicegraph.request.duration",
	} {
		parsed, ok := Parse(raw, nil)
		require.True(t, ok)
		require.Equal(t, canonical, parsed.CanonicalName)
	}
}
