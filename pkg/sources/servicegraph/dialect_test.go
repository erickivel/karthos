package servicegraph

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestDialectTranslatesDerivedServiceGraphMetrics(t *testing.T) {
	for raw, canonical := range map[string]string{
		"traces_service_graph_request_total":  "servicegraph.request.count",
		"traces_service_graph_request_server": "servicegraph.request.duration",
	} {
		got, ok := Dialect.Translate(raw)
		require.True(t, ok)
		require.Equal(t, canonical, got)
	}
}
