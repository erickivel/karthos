package otlp

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sync"
	"time"

	backendingest "github.com/erickivel/karthos/pkg/backend/ingest"
	"github.com/erickivel/karthos/pkg/core/logging"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/status"
)

type Config struct {
	GRPCListenAddress   string
	HealthListenAddress string
	EnabledSignals      []string
}

type Server struct {
	builder      *backendingest.EnvelopeBuilder
	router       *backendingest.SignalRouter
	logger       *logging.Logger
	grpcServer   *grpc.Server
	healthServer *http.Server
	grpcListener net.Listener
	healthListen net.Listener
	enabled      map[string]bool
}

func NewServer(config Config, builder *backendingest.EnvelopeBuilder, router *backendingest.SignalRouter, logger *logging.Logger) (*Server, error) {
	if builder == nil {
		return nil, fmt.Errorf("envelope builder is required")
	}
	if router == nil {
		return nil, fmt.Errorf("signal router is required")
	}
	if logger == nil {
		var err error
		logger, err = logging.New("otlp-server", logging.DefaultLevel)
		if err != nil {
			return nil, err
		}
	}

	grpcListener, err := net.Listen("tcp", config.GRPCListenAddress)
	if err != nil {
		return nil, fmt.Errorf("listen grpc: %w", err)
	}
	healthListener, err := net.Listen("tcp", config.HealthListenAddress)
	if err != nil {
		_ = grpcListener.Close()
		return nil, fmt.Errorf("listen health: %w", err)
	}

	server := &Server{
		builder:      builder,
		router:       router,
		logger:       logger,
		grpcListener: grpcListener,
		healthListen: healthListener,
		enabled:      make(map[string]bool, len(config.EnabledSignals)),
	}
	for _, signal := range config.EnabledSignals {
		server.enabled[signal] = true
	}

	server.grpcServer = grpc.NewServer()
	colmetricpb.RegisterMetricsServiceServer(server.grpcServer, &metricsService{server: server})
	coltracepb.RegisterTraceServiceServer(server.grpcServer, &traceService{server: server})

	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	server.healthServer = &http.Server{Handler: healthMux, ReadHeaderTimeout: 10 * time.Second}

	return server, nil
}

func (s *Server) GRPCAddress() string {
	return s.grpcListener.Addr().String()
}

func (s *Server) HealthAddress() string {
	return s.healthListen.Addr().String()
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 2)
	var wg sync.WaitGroup

	wg.Add(2)
	go s.serveGRPC(&wg, errCh)
	go s.serveHealth(&wg, errCh)

	select {
	case <-ctx.Done():
		s.shutdown()
	case err := <-errCh:
		s.shutdown()
		wg.Wait()
		if isExpectedShutdownError(err) {
			return nil
		}
		return err
	}

	wg.Wait()

	select {
	case err := <-errCh:
		if isExpectedShutdownError(err) {
			return nil
		}
		return err
	default:
		return nil
	}
}

func isExpectedShutdownError(err error) bool {
	return errors.Is(err, http.ErrServerClosed) || errors.Is(err, grpc.ErrServerStopped)
}

func (s *Server) serveGRPC(wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()
	if err := s.grpcServer.Serve(s.grpcListener); err != nil {
		errCh <- err
	}
}

func (s *Server) serveHealth(wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()
	if err := s.healthServer.Serve(s.healthListen); err != nil {
		errCh <- err
	}
}

func (s *Server) shutdown() {
	s.grpcServer.GracefulStop()
	_ = s.healthServer.Shutdown(context.Background())
}

type metricsService struct {
	colmetricpb.UnimplementedMetricsServiceServer
	server *Server
}

func (m *metricsService) Export(ctx context.Context, request *colmetricpb.ExportMetricsServiceRequest) (*colmetricpb.ExportMetricsServiceResponse, error) {
	if !m.server.enabled["metric"] {
		return nil, status.Error(codes.Unimplemented, "metric signal disabled")
	}
	envelopes, err := m.server.builder.BuildMetricEnvelopes("otlp/grpc", request)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	for _, envelope := range envelopes {
		if err := m.server.router.Route(ctx, envelope); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	return &colmetricpb.ExportMetricsServiceResponse{}, nil
}

type traceService struct {
	coltracepb.UnimplementedTraceServiceServer
	server *Server
}

func (t *traceService) Export(ctx context.Context, request *coltracepb.ExportTraceServiceRequest) (*coltracepb.ExportTraceServiceResponse, error) {
	if !t.server.enabled["trace"] {
		return nil, status.Error(codes.Unimplemented, "trace signal disabled")
	}
	envelopes, err := t.server.builder.BuildTraceEnvelopes("otlp/grpc", request)
	if err != nil {
		return nil, status.Error(codes.InvalidArgument, err.Error())
	}
	for _, envelope := range envelopes {
		if err := t.server.router.Route(ctx, envelope); err != nil {
			return nil, status.Error(codes.Internal, err.Error())
		}
	}
	return &coltracepb.ExportTraceServiceResponse{}, nil
}
