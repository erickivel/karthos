package istio

import (
	"testing"

	"github.com/stretchr/testify/require"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"

	"github.com/erickivel/karthos/pkg/core/semconv"
)

func TestCallsEdgeMetadataClassifiesIstioMetricsAndPreservesMeshDimensions(t *testing.T) {
	attrs := []*commonpb.KeyValue{
		{Key: semconv.AttrNetworkProtocolName, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "http"}}},
		{Key: semconv.AttrMeshPolicy, Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "mutual_tls"}}},
	}

	metadata, ok := CallsEdgeMetadata("istio_requests_total", "servicegraph.request.count", attrs)
	require.True(t, ok)
	require.Equal(t, map[string]string{
		semconv.AttrNetworkProtocolName: "http",
		semconv.AttrMeshPolicy:          "mutual_tls",
	}, metadata)
}
