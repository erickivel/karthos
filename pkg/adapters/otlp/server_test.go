package otlp

import (
	"bytes"
	"context"
	"io"
	"net/http"
	"testing"
	"time"

	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	commonpb "go.opentelemetry.io/proto/otlp/common/v1"
	metricpb "go.opentelemetry.io/proto/otlp/metrics/v1"
	resourcepb "go.opentelemetry.io/proto/otlp/resource/v1"
	tracepb "go.opentelemetry.io/proto/otlp/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/encoding/gzip"
	"google.golang.org/protobuf/proto"

	"github.com/stretchr/testify/require"

	backendingest "github.com/erickivel/oksg/pkg/backend/ingest"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/core/schema"
)

func TestServerAcceptsMetricsOverGRPC(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	metricsSink := &recordingIngestor{}
	logger, err := logging.NewWithWriter("otlp-server-test", "debug", io.Discard)
	require.NoError(t, err)

	server, err := NewServer(Config{
		GRPCListenAddress:   "127.0.0.1:0",
		HTTPListenAddress:   "127.0.0.1:0",
		HealthListenAddress: "127.0.0.1:0",
		EnabledSignals:      []string{"metric"},
	}, backendingest.NewEnvelopeBuilder(fixedClock(time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC))), backendingest.NewSignalRouter(metricsSink, nil, nil), logger)
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx)
	}()
	defer func() {
		cancel()
		require.NoError(t, <-errCh)
	}()

	conn, err := grpc.NewClient(server.GRPCAddress(), grpc.WithTransportCredentials(insecure.NewCredentials()))
	require.NoError(t, err)
	defer func() {
		require.NoError(t, conn.Close())
	}()

	client := colmetricpb.NewMetricsServiceClient(conn)
	_, err = client.Export(context.Background(), &colmetricpb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricpb.ResourceMetrics{
			{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "gateway"}}},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(metricsSink.envelopes) == 1 }, 2*time.Second, 50*time.Millisecond)
	require.Equal(t, schema.SignalMetric, metricsSink.envelopes[0].SignalType)
}

func TestServerAcceptsGzipCompressedMetricsOverGRPC(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	metricsSink := &recordingIngestor{}
	logger, err := logging.NewWithWriter("otlp-server-test", "debug", io.Discard)
	require.NoError(t, err)

	server, err := NewServer(Config{
		GRPCListenAddress:   "127.0.0.1:0",
		HTTPListenAddress:   "127.0.0.1:0",
		HealthListenAddress: "127.0.0.1:0",
		EnabledSignals:      []string{"metric"},
	}, backendingest.NewEnvelopeBuilder(fixedClock(time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC))), backendingest.NewSignalRouter(metricsSink, nil, nil), logger)
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx)
	}()
	defer func() {
		cancel()
		require.NoError(t, <-errCh)
	}()

	conn, err := grpc.NewClient(
		server.GRPCAddress(),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithDefaultCallOptions(grpc.UseCompressor(gzip.Name)),
	)
	require.NoError(t, err)
	defer func() {
		require.NoError(t, conn.Close())
	}()

	client := colmetricpb.NewMetricsServiceClient(conn)
	_, err = client.Export(context.Background(), &colmetricpb.ExportMetricsServiceRequest{
		ResourceMetrics: []*metricpb.ResourceMetrics{
			{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "gateway"}}},
					},
				},
			},
		},
	})
	require.NoError(t, err)
	require.Eventually(t, func() bool { return len(metricsSink.envelopes) == 1 }, 2*time.Second, 50*time.Millisecond)
	require.Equal(t, schema.SignalMetric, metricsSink.envelopes[0].SignalType)
}

func TestServerAcceptsTracesOverHTTP(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	tracesSink := &recordingIngestor{}
	logger, err := logging.NewWithWriter("otlp-server-test", "debug", io.Discard)
	require.NoError(t, err)

	server, err := NewServer(Config{
		GRPCListenAddress:   "127.0.0.1:0",
		HTTPListenAddress:   "127.0.0.1:0",
		HealthListenAddress: "127.0.0.1:0",
		EnabledSignals:      []string{"trace"},
	}, backendingest.NewEnvelopeBuilder(fixedClock(time.Date(2026, 3, 14, 12, 0, 0, 0, time.UTC))), backendingest.NewSignalRouter(nil, tracesSink, nil), logger)
	require.NoError(t, err)

	errCh := make(chan error, 1)
	go func() {
		errCh <- server.Run(ctx)
	}()
	defer func() {
		cancel()
		require.NoError(t, <-errCh)
	}()

	payload, err := proto.Marshal(&coltracepb.ExportTraceServiceRequest{
		ResourceSpans: []*tracepb.ResourceSpans{
			{
				Resource: &resourcepb.Resource{
					Attributes: []*commonpb.KeyValue{
						{Key: "service.name", Value: &commonpb.AnyValue{Value: &commonpb.AnyValue_StringValue{StringValue: "gateway"}}},
					},
				},
			},
		},
	})
	require.NoError(t, err)

	response, err := http.Post("http://"+server.HTTPAddress()+"/v1/traces", "application/x-protobuf", bytes.NewReader(payload))
	require.NoError(t, err)
	defer func() {
		require.NoError(t, response.Body.Close())
	}()

	require.Equal(t, http.StatusOK, response.StatusCode)
	require.Eventually(t, func() bool { return len(tracesSink.envelopes) == 1 }, 2*time.Second, 50*time.Millisecond)
	require.Equal(t, schema.SignalTrace, tracesSink.envelopes[0].SignalType)
}

type recordingIngestor struct {
	envelopes []schema.RawEnvelope
}

func fixedClock(now time.Time) func() time.Time {
	return func() time.Time { return now }
}

func (r *recordingIngestor) Ingest(_ context.Context, envelope schema.RawEnvelope) error {
	r.envelopes = append(r.envelopes, envelope)
	return nil
}
