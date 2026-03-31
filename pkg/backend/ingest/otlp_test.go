package ingest

import (
	"context"
	"testing"
	"time"

	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"

	"github.com/stretchr/testify/require"

	"github.com/erickivel/oksg/pkg/core/schema"
)

func TestBuildMetricEnvelopesFromOTLPRequest(t *testing.T) {
	builder := NewEnvelopeBuilder(time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC))

	request := &colmetricpb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricpb.ResourceMetrics{
			{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "gateway"}}},
						{Key: "k8s.namespace.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "oksg-demo"}}},
					},
				},
				ScopeMetrics: []*metricpb.ScopeMetrics{
					{
						Metrics: []*metricpb.Metric{
							{Name: "http.server.duration"},
						},
					},
				},
			},
		},
	}

	envelopes, err := builder.BuildMetricEnvelopes("otlp/grpc", request)
	require.NoError(t, err)
	require.Len(t, envelopes, 1)
	require.Equal(t, schema.SignalMetric, envelopes[0].SignalType)
	require.Equal(t, "otlp/grpc", envelopes[0].Source)
	require.Equal(t, "gateway", envelopes[0].Resource["service.name"])
	require.Equal(t, "oksg-demo", envelopes[0].Resource["k8s.namespace.name"])
	require.NotEmpty(t, envelopes[0].Payload)
	require.Equal(t, builder.now, envelopes[0].Timestamp)
}

func TestBuildTraceEnvelopesFromOTLPRequest(t *testing.T) {
	builder := NewEnvelopeBuilder(time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC))

	request := &coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{
			{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "gateway"}}},
					},
				},
				ScopeSpans: []*tracepb.ScopeSpans{
					{
						Spans: []*tracepb.Span{
							{Name: "GET /health"},
						},
					},
				},
			},
		},
	}

	envelopes, err := builder.BuildTraceEnvelopes("otlp/http", request)
	require.NoError(t, err)
	require.Len(t, envelopes, 1)
	require.Equal(t, schema.SignalTrace, envelopes[0].SignalType)
	require.Equal(t, "gateway", envelopes[0].Resource["service.name"])
	require.NotEmpty(t, envelopes[0].Payload)
}

func TestSignalRouterRoutesBySignalType(t *testing.T) {
	metricsSink := &recordingIngestor{}
	tracesSink := &recordingIngestor{}
	router := NewSignalRouter(metricsSink, tracesSink, nil)

	metricEnvelope, err := schema.NewRawEnvelope(
		"otlp/grpc",
		schema.SignalMetric,
		map[string]string{"service.name": "gateway"},
		[]byte(`{"metric":"cpu"}`),
		time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)

	traceEnvelope, err := schema.NewRawEnvelope(
		"otlp/grpc",
		schema.SignalTrace,
		map[string]string{"service.name": "gateway"},
		[]byte(`{"span":"GET /health"}`),
		time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC),
	)
	require.NoError(t, err)

	require.NoError(t, router.Route(context.Background(), metricEnvelope))
	require.NoError(t, router.Route(context.Background(), traceEnvelope))

	require.Len(t, metricsSink.envelopes, 1)
	require.Len(t, tracesSink.envelopes, 1)
	require.Equal(t, schema.SignalMetric, metricsSink.envelopes[0].SignalType)
	require.Equal(t, schema.SignalTrace, tracesSink.envelopes[0].SignalType)
}

type recordingIngestor struct {
	envelopes []schema.RawEnvelope
}

func (r *recordingIngestor) Ingest(_ context.Context, envelope schema.RawEnvelope) error {
	r.envelopes = append(r.envelopes, envelope)
	return nil
}
