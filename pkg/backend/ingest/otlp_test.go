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

	"github.com/erickivel/karthos/pkg/core/schema"
)

func TestBuildMetricEnvelopesFromOTLPRequest(t *testing.T) {
	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	builder := NewEnvelopeBuilder(func() time.Time { return now })

	request := &colmetricpb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricpb.ResourceMetrics{
			{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "gateway"}}},
						{Key: "k8s.namespace.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "karthos-demo"}}},
						{Key: "k8s.pod.uid", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "pod-uid-a"}}},
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
	require.Equal(t, "karthos-demo", envelopes[0].Resource["k8s.namespace.name"])
	require.Equal(t, "pod-uid-a", envelopes[0].Resource["k8s.pod.uid"])
	require.NotEmpty(t, envelopes[0].Payload)
	require.Equal(t, now, envelopes[0].Timestamp)
}

func TestBuildMetricEnvelopesUsesClockForEachRequest(t *testing.T) {
	first := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	second := first.Add(2 * time.Minute)
	calls := 0
	builder := NewEnvelopeBuilder(func() time.Time {
		calls++
		if calls == 1 {
			return first
		}
		return second
	})

	request := &colmetricpb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricpb.ResourceMetrics{
			{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "gateway"}}},
					},
				},
			},
		},
	}

	firstEnvelopes, err := builder.BuildMetricEnvelopes("otlp/grpc", request)
	require.NoError(t, err)
	secondEnvelopes, err := builder.BuildMetricEnvelopes("otlp/grpc", request)
	require.NoError(t, err)

	require.Equal(t, first, firstEnvelopes[0].Timestamp)
	require.Equal(t, second, secondEnvelopes[0].Timestamp)
}

func TestBuildTraceEnvelopesFromOTLPRequest(t *testing.T) {
	now := time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC)
	builder := NewEnvelopeBuilder(func() time.Time { return now })

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

	envelopes, err := builder.BuildTraceEnvelopes("test-source", request)
	require.NoError(t, err)
	require.Len(t, envelopes, 1)
	require.Equal(t, schema.SignalTrace, envelopes[0].SignalType)
	require.Equal(t, "gateway", envelopes[0].Resource["service.name"])
	require.NotEmpty(t, envelopes[0].Payload)
	require.Equal(t, now, envelopes[0].Timestamp)
}

func TestSignalRouterRoutesBySignalType(t *testing.T) {
	metricsSink := &recordingIngestor{}
	tracesSink := &recordingIngestor{}
	router := NewSignalRouter(metricsSink, tracesSink)

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
