package storage

import (
	"context"
	"fmt"
	"io"

	"github.com/erickivel/karthos/pkg/adapters/storage/postgresql"
	"github.com/erickivel/karthos/pkg/adapters/storage/victoriametrics"
	"github.com/erickivel/karthos/pkg/backend/persistence"
	"github.com/erickivel/karthos/pkg/sources"
)

// Stores bundles all store implementations created by the factory.
// The caller uses these to wire the ingest and query pipelines.
type Stores struct {
	MetricStore    persistence.MetricStore
	GraphStore     persistence.GraphStore
	MetadataStore  persistence.MetadataStore
	EntityRegistry persistence.EntityRegistry
	closers        []io.Closer
}

// Close closes all closeable resources (connection pools, HTTP clients).
func (s *Stores) Close() error {
	var firstErr error
	for _, c := range s.closers {
		if err := c.Close(); err != nil && firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// NewStores creates the appropriate store implementations based on config.
func NewStores(ctx context.Context, cfg StorageConfig) (*Stores, error) {
	switch cfg.Backend {
	case BackendMemory:
		return newMemoryStores(cfg), nil
	case BackendExternal:
		return newExternalStores(ctx, cfg)
	default:
		return nil, fmt.Errorf("unsupported storage backend: %s", cfg.Backend)
	}
}

func newMemoryStores(cfg StorageConfig) *Stores {
	var graphOpts []persistence.InMemoryGraphStoreOption
	if cfg.NodeTTL > 0 {
		graphOpts = append(graphOpts, persistence.WithInMemoryNodeTTL(cfg.NodeTTL))
	}
	s := &Stores{
		MetricStore:    persistence.DecorateMetricStore(persistence.NewInMemoryMetricStore(persistence.WithMetricQueryViews(sources.DefaultRegistry().QueryViews))),
		GraphStore:     persistence.NewInMemoryGraphStore(graphOpts...),
		MetadataStore:  persistence.NewInMemoryMetadataStore(),
		EntityRegistry: persistence.NewInMemoryEntityRegistry(),
	}
	return s
}

func newExternalStores(ctx context.Context, cfg StorageConfig) (*Stores, error) {
	if cfg.PostgresDSN == "" {
		return nil, fmt.Errorf("postgres-dsn is required when storage backend is %q", BackendExternal)
	}
	if cfg.VMURL == "" {
		return nil, fmt.Errorf("victoriametrics-url is required when storage backend is %q", BackendExternal)
	}

	// PostgreSQL connection + migration.
	pgConn, err := postgresql.NewConn(ctx, cfg.PostgresDSN)
	if err != nil {
		return nil, fmt.Errorf("external stores: %w", err)
	}
	if err := pgConn.Ping(ctx); err != nil {
		_ = pgConn.Close()
		return nil, fmt.Errorf("external stores postgres ping: %w", err)
	}
	if err := postgresql.Migrate(ctx, pgConn); err != nil {
		_ = pgConn.Close()
		return nil, fmt.Errorf("external stores: %w", err)
	}

	// VictoriaMetrics HTTP client.
	vmStore := victoriametrics.NewVMMetricStore(cfg.VMURL)
	if err := vmStore.Ping(ctx); err != nil {
		_ = pgConn.Close()
		return nil, fmt.Errorf("external stores vm ping: %w", err)
	}

	var pgGraphOpts []postgresql.PostgresGraphStoreOption
	if cfg.NodeTTL > 0 {
		pgGraphOpts = append(pgGraphOpts, postgresql.WithPostgresNodeTTL(cfg.NodeTTL))
	}
	s := &Stores{
		MetricStore:    persistence.DecorateMetricStore(vmStore),
		GraphStore:     postgresql.NewPostgresGraphStore(pgConn, pgGraphOpts...),
		MetadataStore:  postgresql.NewPostgresMetadataStore(pgConn),
		EntityRegistry: postgresql.NewPostgresEntityRegistry(pgConn),
		closers:        []io.Closer{pgConn, vmStore},
	}
	return s, nil
}
