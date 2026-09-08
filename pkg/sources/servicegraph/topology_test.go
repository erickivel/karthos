package servicegraph

import (
	"testing"

	"github.com/stretchr/testify/require"
)

func TestCallsEdgeMetadataClassifiesDerivedServiceGraphMetrics(t *testing.T) {
	metadata, ok := CallsEdgeMetadata("traces_service_graph_request_total", "servicegraph.request.count", nil)
	require.True(t, ok)
	require.Empty(t, metadata)
}
