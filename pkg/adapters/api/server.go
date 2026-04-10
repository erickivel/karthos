package api

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"time"

	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/logging"
)

// Config holds the API server configuration. All store fields are interfaces;
// the API package never imports concrete storage implementations.
type Config struct {
	ListenAddress      string
	MetricStore        persistence.MetricStore
	GraphStore         persistence.GraphStore
	MetadataStore      persistence.MetadataStore
	EntityRegistry     persistence.EntityRegistry
	StatusStaleSeconds int // default 300 if zero
}

// Server is the REST API server for query and control endpoints.
type Server struct {
	config     Config
	logger     *logging.Logger
	httpServer *http.Server
	listener   net.Listener
	startTime  time.Time
}

// NewServer creates a new API server. The listener is opened eagerly so port 0
// can be resolved for tests via Address().
func NewServer(cfg Config, logger *logging.Logger) (*Server, error) {
	if logger == nil {
		var err error
		logger, err = logging.New("api-server", logging.DefaultLevel)
		if err != nil {
			return nil, err
		}
	}

	listener, err := net.Listen("tcp", cfg.ListenAddress)
	if err != nil {
		return nil, fmt.Errorf("listen api: %w", err)
	}

	now := time.Now()
	mux := http.NewServeMux()

	s := &Server{
		config:    cfg,
		logger:    logger,
		listener:  listener,
		startTime: now,
	}

	staleSeconds := cfg.StatusStaleSeconds
	if staleSeconds <= 0 {
		staleSeconds = 300
	}

	statusHandler := &StatusHandler{
		metadataStore:    cfg.MetadataStore,
		graphStore:       cfg.GraphStore,
		startTime:        now,
		hasMetricStore:   cfg.MetricStore != nil,
		hasGraphStore:    cfg.GraphStore != nil,
		hasMetadataStore: cfg.MetadataStore != nil,
		hasEntityReg:     cfg.EntityRegistry != nil,
		staleThreshold:   time.Duration(staleSeconds) * time.Second,
		logger:           logger,
	}
	statusHandler.RegisterRoutes(mux)

	graphHandler := &GraphHandler{
		graphStore:     cfg.GraphStore,
		entityRegistry: cfg.EntityRegistry,
		logger:         logger,
	}
	graphHandler.RegisterRoutes(mux)

	metricsHandler := newMetricsHandler(cfg.MetricStore, logger)
	metricsHandler.RegisterRoutes(mux)

	entityHandler := &EntityHandler{
		entityRegistry: cfg.EntityRegistry,
		logger:         logger,
	}
	entityHandler.RegisterRoutes(mux)

	s.httpServer = &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: 10 * time.Second,
	}

	return s, nil
}

// Address returns the resolved listen address (useful when bound to port 0).
func (s *Server) Address() string {
	return s.listener.Addr().String()
}

// Run serves HTTP requests until the context is cancelled, then shuts down
// gracefully.
func (s *Server) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		if err := s.httpServer.Serve(s.listener); err != nil {
			errCh <- err
		}
	}()

	select {
	case <-ctx.Done():
		_ = s.httpServer.Shutdown(context.Background())
		return nil
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}
