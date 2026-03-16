package otlp

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"sync"
	"time"

	backendingest "github.com/erickivel/oksg/pkg/backend/ingest"
	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/logging"
	colmetricpb "go.opentelemetry.io/proto/otlp/collector/metrics/v1"
	coltracepb "go.opentelemetry.io/proto/otlp/collector/trace/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	_ "google.golang.org/grpc/encoding/gzip"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
)

type Config struct {
	GRPCListenAddress   string
	HTTPListenAddress   string
	HealthListenAddress string
	EnabledSignals      []string
	MetricStore         persistence.MetricStore // optional; enables /debug/metrics when set
}

type Server struct {
	config       Config
	builder      *backendingest.EnvelopeBuilder
	router       *backendingest.SignalRouter
	logger       *logging.Logger
	grpcServer   *grpc.Server
	httpServer   *http.Server
	healthServer *http.Server
	grpcListener net.Listener
	httpListener net.Listener
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
	httpListener, err := net.Listen("tcp", config.HTTPListenAddress)
	if err != nil {
		_ = grpcListener.Close()
		return nil, fmt.Errorf("listen http: %w", err)
	}
	healthListener, err := net.Listen("tcp", config.HealthListenAddress)
	if err != nil {
		_ = grpcListener.Close()
		_ = httpListener.Close()
		return nil, fmt.Errorf("listen health: %w", err)
	}

	server := &Server{
		config:       config,
		builder:      builder,
		router:       router,
		logger:       logger,
		grpcListener: grpcListener,
		httpListener: httpListener,
		healthListen: healthListener,
		enabled:      make(map[string]bool, len(config.EnabledSignals)),
	}
	for _, signal := range config.EnabledSignals {
		server.enabled[signal] = true
	}

	server.grpcServer = grpc.NewServer()
	colmetricpb.RegisterMetricsServiceServer(server.grpcServer, &metricsService{server: server})
	coltracepb.RegisterTraceServiceServer(server.grpcServer, &traceService{server: server})

	mux := http.NewServeMux()
	mux.HandleFunc("/v1/metrics", server.handleMetricsHTTP)
	mux.HandleFunc("/v1/traces", server.handleTracesHTTP)
	server.httpServer = &http.Server{Handler: mux, ReadHeaderTimeout: 10 * time.Second}

	healthMux := http.NewServeMux()
	healthMux.HandleFunc("/healthz", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	if config.MetricStore != nil {
		ms := config.MetricStore
		healthMux.HandleFunc("/debug/metrics", func(w http.ResponseWriter, r *http.Request) {
			samples, err := ms.Query(r.Context(), query.MetricQuery{})
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(samples)
		})
	}
	server.healthServer = &http.Server{Handler: healthMux, ReadHeaderTimeout: 10 * time.Second}

	return server, nil
}

func (s *Server) GRPCAddress() string {
	return s.grpcListener.Addr().String()
}

func (s *Server) HTTPAddress() string {
	return s.httpListener.Addr().String()
}

func (s *Server) HealthAddress() string {
	return s.healthListen.Addr().String()
}

func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 3)
	var wg sync.WaitGroup

	wg.Add(3)
	go s.serveGRPC(&wg, errCh)
	go s.serveHTTP(&wg, errCh)
	go s.serveHealth(&wg, errCh)

	select {
	case <-ctx.Done():
		s.shutdown()
	case err := <-errCh:
		s.shutdown()
		wg.Wait()
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}

	wg.Wait()

	select {
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	default:
		return nil
	}
}

func (s *Server) serveGRPC(wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()
	if err := s.grpcServer.Serve(s.grpcListener); err != nil {
		errCh <- err
	}
}

func (s *Server) serveHTTP(wg *sync.WaitGroup, errCh chan<- error) {
	defer wg.Done()
	if err := s.httpServer.Serve(s.httpListener); err != nil {
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
	_ = s.httpServer.Shutdown(context.Background())
	_ = s.healthServer.Shutdown(context.Background())
}

func (s *Server) handleMetricsHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.enabled["metric"] {
		http.Error(w, "metric signal disabled", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	request := &colmetricpb.ExportMetricsServiceRequest{}
	if err := proto.Unmarshal(body, request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	envelopes, err := s.builder.BuildMetricEnvelopes("otlp/http", request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, envelope := range envelopes {
		if err := s.router.Route(r.Context(), envelope); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	payload, err := proto.Marshal(&colmetricpb.ExportMetricsServiceResponse{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
}

func (s *Server) handleTracesHTTP(w http.ResponseWriter, r *http.Request) {
	if !s.enabled["trace"] {
		http.Error(w, "trace signal disabled", http.StatusNotFound)
		return
	}

	body, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	request := &coltracepb.ExportTraceServiceRequest{}
	if err := proto.Unmarshal(body, request); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	envelopes, err := s.builder.BuildTraceEnvelopes("otlp/http", request)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	for _, envelope := range envelopes {
		if err := s.router.Route(r.Context(), envelope); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	}

	payload, err := proto.Marshal(&coltracepb.ExportTraceServiceResponse{})
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/x-protobuf")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(payload)
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
