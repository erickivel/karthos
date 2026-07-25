package ingest_test

import (
	"context"

	"testing"
	"time"

	metricspb "go.opentelemetry.io/proto/otlp/metrics/v1"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/core/metrics"
)

func TestOTLPNormalizer_IstioProtocolCountersPreserveCumulativeSums(t *testing.T) {
	tests := []struct {
		raw       string
		canonical string
		unit      metrics.Unit
	}{
		{raw: "istio_request_messages_total", canonical: "servicegraph.grpc.request.message.count", unit: metrics.UnitCount},
		{raw: "istio_response_messages_total", canonical: "servicegraph.grpc.response.message.count", unit: metrics.UnitCount},
		{raw: "istio_tcp_connections_opened_total", canonical: "servicegraph.tcp.connection.open.count", unit: metrics.UnitCount},
		{raw: "istio_tcp_connections_closed_total", canonical: "servicegraph.tcp.connection.close.count", unit: metrics.UnitCount},
		{raw: "istio_tcp_sent_bytes_total", canonical: "servicegraph.tcp.bytes.sent", unit: metrics.UnitBytes},
		{raw: "istio_tcp_received_bytes_total", canonical: "servicegraph.tcp.bytes.received", unit: metrics.UnitBytes},
	}

	for _, tt := range tests {
		t.Run(tt.raw, func(t *testing.T) {
			normalizer := newNormalizer()
			rm := makeSumRMIntWithAttrs(tt.raw, 25, uint64(time.Now().UnixNano()), map[string]string{
				"client":                "mesh-protocol-client",
				"server":                "mesh-protocol-server",
				"client_namespace":      "oksg-demo",
				"server_namespace":      "oksg-demo",
				"network.protocol.name": "grpc",
				"mesh_security_policy":  "mutual_tls",
				"oksg.source":           "mesh.istio",
			}, metricspb.AggregationTemporality_AGGREGATION_TEMPORALITY_CUMULATIVE, true)

			samples, err := normalizer.Normalize(context.Background(), makeEnvelope(t, map[string]string{"k8s.node.name": "worker-1"}, rm))
			require.NoError(t, err)
			require.Len(t, samples, 1)
			sample := samples[0]
			assert.Equal(t, tt.canonical, sample.MetricName)
			assert.Equal(t, 25.0, sample.Value)
			assert.Equal(t, tt.unit, sample.Unit)
			assert.Equal(t, "mesh.istio", sample.Labels["source"])
			assert.Equal(t, "mesh-protocol-client", sample.Labels["edge_client"])
			assert.Equal(t, "mesh-protocol-server", sample.Labels["edge_server"])
			assert.Equal(t, "grpc", sample.Labels["network.protocol.name"])
			assert.Equal(t, "mutual_tls", sample.Labels["mesh.security.policy"])
			assert.NotContains(t, sample.Labels, "temporality")
		})
	}
}

func TestOTLPNormalizer_Istio5xxRequestsAlsoEmitErrorCount(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeGaugeRMWithAttrs("istio_requests_total", 3.0, tsNano, map[string]string{
		"client":                    "gateway-go",
		"server":                    "service-a-python",
		"client_namespace":          "oksg-demo",
		"server_namespace":          "oksg-demo",
		"http.response.status_code": "503",
		"oksg.source":               "mesh.istio",
	})
	env := makeEnvelope(t, map[string]string{"k8s.node.name": "worker-1"}, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 2)

	_, ok := findSample(samples, "servicegraph.request.count")
	require.True(t, ok)
	errSample, ok := findSample(samples, "servicegraph.request.error.count")
	require.True(t, ok)
	assert.Equal(t, "mesh.istio", errSample.Labels["source"])
	assert.Equal(t, "503", errSample.Labels["http.response.status_code"])
}

func TestOTLPNormalizer_IstioDurationMillisecondsHistogramEmitsNanosecondSumCount(t *testing.T) {
	normalizer := newNormalizer()
	tsNano := uint64(time.Now().UnixNano())

	rm := makeHistogramRMWithAttrs("istio_request_duration_milliseconds", 12.5, 5, tsNano, map[string]string{
		"client":           "gateway-go",
		"server":           "service-a-python",
		"client_namespace": "oksg-demo",
		"server_namespace": "oksg-demo",
		"oksg.source":      "mesh.istio",
	})
	env := makeEnvelope(t, map[string]string{"k8s.node.name": "worker-1"}, rm)

	samples, err := normalizer.Normalize(context.Background(), env)
	require.NoError(t, err)
	require.Len(t, samples, 2)

	sumSample, ok := findSample(samples, "servicegraph.request.duration_sum")
	require.True(t, ok)
	assert.InDelta(t, 12_500_000.0, sumSample.Value, 1e-6)
	assert.Equal(t, metrics.UnitNanoseconds, sumSample.Unit)

	countSample, ok := findSample(samples, "servicegraph.request.duration_count")
	require.True(t, ok)
	assert.InDelta(t, 5.0, countSample.Value, 1e-9)
}
