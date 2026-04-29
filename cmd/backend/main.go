package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/erickivel/oksg/pkg/adapters/api"
	"github.com/erickivel/oksg/pkg/adapters/otlp"
	"github.com/erickivel/oksg/pkg/adapters/storage"
	"github.com/erickivel/oksg/pkg/backend/ingest"
	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

const (
	defaultBackendGRPCListenAddr   = "0.0.0.0:4317"
	defaultBackendHTTPListenAddr   = "0.0.0.0:4318"
	defaultBackendHealthListenAddr = "0.0.0.0:13133"
	defaultBackendAPIListenAddr    = "0.0.0.0:8080"
	defaultBackendLogLevel         = "info"
	defaultStorageBackend          = "memory"
	defaultTopologyFlushInterval   = 60 * time.Second
	defaultStatusStaleSeconds      = 300
	defaultNodeTTL                 = 24 * time.Hour
	// defaultDiscoveryRefreshInterval re-runs structural ingestion from the
	// discovery profile on a timer so workload/ingress/configmap/secret/pvc
	// nodes keep producing fresh snapshots and survive `nodeTTL` aging in the
	// graph store. MUST stay strictly less than defaultNodeTTL — otherwise
	// structural nodes age out between refreshes and disappear from the API.
	defaultDiscoveryRefreshInterval = 10 * time.Minute
)

type config struct {
	GRPCListenAddress     string
	HTTPListenAddress     string
	HealthListenAddress   string
	APIListenAddress      string
	LogLevel              string
	EnabledSignals        []string
	ClusterName           string
	DiscoveryProfile      string
	StorageBackend        string
	PostgresDSN           string
	VMURL                 string
	RawStoreEnabled       bool
	TopologyFlushInterval time.Duration
	StatusStaleSeconds    int
	NodeTTL               time.Duration
	// DiscoveryRefreshInterval re-runs structural ingestion from the
	// discovery profile on a timer. Required because the graph store
	// applies NodeTTL based on snapshot creation time; without periodic
	// re-ingest, workload/ingress/configmap/secret/pvc nodes (which only
	// the structural pipeline produces) age out and the API stops
	// returning them. 0 disables the refresh — only do that for tests.
	DiscoveryRefreshInterval time.Duration
}

func main() {
	if err := run(context.Background(), os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func run(ctx context.Context, args []string) error {
	cfg, err := parseConfig(args)
	if err != nil {
		return err
	}

	logger, err := logging.New("backend", cfg.LogLevel)
	if err != nil {
		return err
	}
	if cfg.NodeTTL > 0 && cfg.NodeTTL < cfg.TopologyFlushInterval {
		logger.Warn("node_ttl shorter than flush interval",
			logging.String("node_ttl", cfg.NodeTTL.String()),
			logging.String("topology_flush_interval", cfg.TopologyFlushInterval.String()),
		)
	}

	stores, err := storage.NewStores(ctx, storage.StorageConfig{
		Backend:     storage.StorageBackend(cfg.StorageBackend),
		PostgresDSN: cfg.PostgresDSN,
		VMURL:       cfg.VMURL,
		RawEnabled:  cfg.RawStoreEnabled,
		NodeTTL:     cfg.NodeTTL,
	})
	if err != nil {
		return fmt.Errorf("initialize stores: %w", err)
	}
	defer func() {
		if cerr := stores.Close(); cerr != nil {
			logger.Error("failed to close stores", logging.String("error", cerr.Error()))
		}
	}()

	logger.Info("storage initialized",
		logging.String("backend", cfg.StorageBackend),
	)

	resolver := ingest.NewOTLPEntityResolverWithRegistry(cfg.ClusterName, stores.EntityRegistry)
	dialect := metrics.NewDialectRegistryFromSources(metrics.DefaultLogicalSources())

	var loadedProfile *profile.ClusterProfile
	if cfg.DiscoveryProfile != "" {
		prof, err := ingestDiscoveryProfile(ctx, cfg, stores.GraphStore, stores.MetadataStore, stores.EntityRegistry, logger)
		if err != nil {
			return err
		}
		loadedProfile = prof

		// Re-ingest the discovery profile periodically so structural nodes
		// (workload/ingress/configmap/secret/pvc) get fresh snapshots and
		// don't age out under NodeTTL. See `defaultDiscoveryRefreshInterval`
		// for the rationale; without this, the structural snapshot is pushed
		// once at startup and disappears after NodeTTL.
		if cfg.DiscoveryRefreshInterval > 0 {
			if cfg.NodeTTL > 0 && cfg.DiscoveryRefreshInterval >= cfg.NodeTTL {
				logger.Warn("discovery refresh interval is not strictly less than node TTL; structural nodes will age out between refreshes",
					logging.String("discovery_refresh_interval", cfg.DiscoveryRefreshInterval.String()),
					logging.String("node_ttl", cfg.NodeTTL.String()),
				)
			}
			go runDiscoveryRefreshLoop(ctx, cfg, stores.GraphStore, stores.MetadataStore, stores.EntityRegistry, logger)
		}
	}

	var normalizerOpts []ingest.NormalizerOption
	if loadedProfile != nil {
		assignment, _, err := metrics.Resolve(metrics.DefaultCatalog(), metrics.DefaultLogicalSources(), loadedProfile)
		if err != nil {
			return fmt.Errorf("resolve metric source assignment: %w", err)
		}
		normalizerOpts = append(normalizerOpts, ingest.WithAssignment(assignment))
		logger.Info("metric source assignment loaded",
			logging.Any("metric_count", len(assignment)),
		)
	}

	normalizer := ingest.NewOTLPNormalizer(resolver, dialect, normalizerOpts...)
	traceTopoBuilder := ingest.NewTraceTopologyBuilder(resolver)
	metricTopoBuilder := ingest.NewMetricTopologyBuilder(cfg.ClusterName)
	meshMetricTopoBuilder := ingest.NewMeshMetricTopologyBuilder(dialect, cfg.ClusterName)

	bufferedTopo := ingest.NewBufferedTopologyIngestor(
		map[schema.SignalType]ingest.TopologyBuilder{
			schema.SignalMetric: ingest.NewCompositeTopologyBuilder(metricTopoBuilder, meshMetricTopoBuilder),
			schema.SignalTrace:  traceTopoBuilder,
		},
		stores.GraphStore,
		logger,
		cfg.TopologyFlushInterval,
	)
	defer bufferedTopo.Close()

	router := ingest.NewSignalRouter(
		ingest.NewMultiIngestor(
			ingest.NewNormalizingIngestor(normalizer, stores.MetricStore, logger),
			bufferedTopo,
		),
		bufferedTopo,
		stores.RawStore,
	)

	logger.Info("normalization pipeline configured",
		logging.String("cluster_name", cfg.ClusterName),
	)

	apiServer, err := api.NewServer(api.Config{
		ListenAddress:      cfg.APIListenAddress,
		MetricStore:        stores.MetricStore,
		GraphStore:         stores.GraphStore,
		MetadataStore:      stores.MetadataStore,
		EntityRegistry:     stores.EntityRegistry,
		StatusStaleSeconds: cfg.StatusStaleSeconds,
	}, logger)
	if err != nil {
		return fmt.Errorf("create api server: %w", err)
	}

	server, err := otlp.NewServer(otlp.Config{
		GRPCListenAddress:   cfg.GRPCListenAddress,
		HTTPListenAddress:   cfg.HTTPListenAddress,
		HealthListenAddress: cfg.HealthListenAddress,
		EnabledSignals:      cfg.EnabledSignals,
		MetricStore:         stores.MetricStore,
		GraphStore:          stores.GraphStore,
	}, ingest.NewEnvelopeBuilder(contextNow), router, logger)
	if err != nil {
		return err
	}

	logger.Info("backend server configured",
		logging.String("grpc_listen_address", server.GRPCAddress()),
		logging.String("http_listen_address", server.HTTPAddress()),
		logging.String("health_listen_address", server.HealthAddress()),
		logging.String("api_listen_address", apiServer.Address()),
		logging.Any("enabled_signals", cfg.EnabledSignals),
	)

	apiErrCh := make(chan error, 1)
	go func() { apiErrCh <- apiServer.Run(ctx) }()

	if err := server.Run(ctx); err != nil {
		return err
	}

	select {
	case err := <-apiErrCh:
		return err
	default:
		return nil
	}
}

func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("backend", flag.ContinueOnError)
	grpcListenAddress := fs.String("grpc-listen-address", envOrDefault("OKSG_BACKEND_GRPC_LISTEN_ADDRESS", defaultBackendGRPCListenAddr), "OTLP gRPC listen address")
	httpListenAddress := fs.String("http-listen-address", envOrDefault("OKSG_BACKEND_HTTP_LISTEN_ADDRESS", defaultBackendHTTPListenAddr), "OTLP HTTP listen address")
	healthListenAddress := fs.String("health-listen-address", envOrDefault("OKSG_BACKEND_HEALTH_LISTEN_ADDRESS", defaultBackendHealthListenAddr), "health listen address")
	apiListenAddress := fs.String("api-listen-address", envOrDefault("OKSG_BACKEND_API_LISTEN_ADDRESS", defaultBackendAPIListenAddr), "REST API listen address")
	logLevel := fs.String("log-level", envOrDefault("OKSG_LOG_LEVEL", defaultBackendLogLevel), "structured log level")
	enabledSignals := fs.String("enabled-signals", envOrDefault("OKSG_BACKEND_ENABLED_SIGNALS", "metric,trace"), "comma-separated enabled signals")
	clusterName := fs.String("cluster-name", envOrDefault("OKSG_CLUSTER_NAME", "default"), "cluster name for entity resolution")
	discoveryProfile := fs.String("discovery-profile", envOrDefault("OKSG_DISCOVERY_PROFILE", ""), "path to ClusterProfile JSON for structural topology")
	storageBackend := fs.String("storage-backend", envOrDefault("OKSG_STORAGE_BACKEND", defaultStorageBackend), "storage backend: memory or external")
	postgresDSN := fs.String("postgres-dsn", envOrDefault("OKSG_POSTGRES_DSN", ""), "PostgreSQL connection string")
	vmURL := fs.String("victoriametrics-url", envOrDefault("OKSG_VICTORIAMETRICS_URL", ""), "VictoriaMetrics base URL")
	rawStoreEnabled := fs.Bool("raw-store-enabled", envOrDefault("OKSG_RAW_STORE_ENABLED", "false") == "true", "enable raw envelope persistence")
	topologyFlushIntervalStr := fs.String("topology-flush-interval", envOrDefault("OKSG_TOPOLOGY_FLUSH_INTERVAL", defaultTopologyFlushInterval.String()), "topology snapshot flush interval")
	statusStaleSeconds := fs.Int("status-stale-seconds", parseIntOrDefault(envOrDefault("OKSG_BACKEND_STATUS_STALE_SECONDS", ""), defaultStatusStaleSeconds), "snapshot staleness threshold in seconds for status endpoint")
	nodeTTLStr := fs.String("node-ttl", envOrDefault("OKSG_NODE_TTL", defaultNodeTTL.String()), "topology node TTL; nodes absent from snapshots for longer than this age out (0 disables)")
	discoveryRefreshStr := fs.String("discovery-refresh-interval", envOrDefault("OKSG_DISCOVERY_REFRESH_INTERVAL", defaultDiscoveryRefreshInterval.String()), "interval to re-ingest the discovery profile so structural topology nodes (workload/ingress/configmap/secret/pvc) refresh under node-ttl; 0 disables")
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	signals, err := parseSignals(*enabledSignals)
	if err != nil {
		return config{}, err
	}

	topologyFlushInterval, err := time.ParseDuration(*topologyFlushIntervalStr)
	if err != nil {
		return config{}, fmt.Errorf("parse topology flush interval: %w", err)
	}
	if topologyFlushInterval <= 0 {
		topologyFlushInterval = defaultTopologyFlushInterval
	}

	nodeTTL, err := time.ParseDuration(*nodeTTLStr)
	if err != nil {
		return config{}, fmt.Errorf("parse node ttl: %w", err)
	}
	if nodeTTL < 0 {
		nodeTTL = 0
	}

	discoveryRefresh, err := time.ParseDuration(*discoveryRefreshStr)
	if err != nil {
		return config{}, fmt.Errorf("parse discovery refresh interval: %w", err)
	}
	if discoveryRefresh < 0 {
		discoveryRefresh = 0
	}

	return config{
		GRPCListenAddress:        *grpcListenAddress,
		HTTPListenAddress:        *httpListenAddress,
		HealthListenAddress:      *healthListenAddress,
		APIListenAddress:         *apiListenAddress,
		LogLevel:                 *logLevel,
		EnabledSignals:           signals,
		ClusterName:              *clusterName,
		DiscoveryProfile:         *discoveryProfile,
		StorageBackend:           *storageBackend,
		PostgresDSN:              *postgresDSN,
		VMURL:                    *vmURL,
		RawStoreEnabled:          *rawStoreEnabled,
		TopologyFlushInterval:    topologyFlushInterval,
		StatusStaleSeconds:       *statusStaleSeconds,
		NodeTTL:                  nodeTTL,
		DiscoveryRefreshInterval: discoveryRefresh,
	}, nil
}

func parseIntOrDefault(s string, def int) int {
	if s == "" {
		return def
	}
	n, err := strconv.Atoi(s)
	if err != nil {
		return def
	}
	return n
}

func parseSignals(input string) ([]string, error) {
	var signals []string
	for _, part := range strings.Split(input, ",") {
		signal := strings.TrimSpace(part)
		if signal == "" {
			continue
		}
		switch signal {
		case "metric", "trace":
			signals = append(signals, signal)
		default:
			return nil, fmt.Errorf("unsupported signal: %s", signal)
		}
	}

	if len(signals) == 0 {
		return nil, fmt.Errorf("at least one signal must be enabled")
	}

	return signals, nil
}

func envOrDefault(key, fallback string) string {
	value := strings.TrimSpace(os.Getenv(key))
	if value == "" {
		return fallback
	}
	return value
}

func contextNow() time.Time {
	return time.Now().UTC()
}

// runDiscoveryRefreshLoop re-runs structural ingestion on the configured
// interval. Required because the graph store applies NodeTTL based on snapshot
// creation time — without periodic re-ingest, workload/ingress/configmap/
// secret/pvc nodes (only produced by the structural pipeline) age out and the
// API stops returning them. Errors are logged and the loop continues; a
// transient discovery-profile read failure shouldn't take the backend down.
func runDiscoveryRefreshLoop(
	ctx context.Context,
	cfg config,
	graphStore persistence.GraphStore,
	metadataStore persistence.MetadataStore,
	entityRegistry persistence.EntityRegistry,
	logger *logging.Logger,
) {
	ticker := time.NewTicker(cfg.DiscoveryRefreshInterval)
	defer ticker.Stop()
	logger.Info("discovery refresh loop started",
		logging.String("interval", cfg.DiscoveryRefreshInterval.String()),
		logging.String("profile_path", cfg.DiscoveryProfile),
	)
	for {
		select {
		case <-ctx.Done():
			logger.Info("discovery refresh loop stopping")
			return
		case <-ticker.C:
			if _, err := ingestDiscoveryProfile(ctx, cfg, graphStore, metadataStore, entityRegistry, logger); err != nil {
				logger.Warn("discovery refresh ingest failed", logging.Error(err))
			}
		}
	}
}

func ingestDiscoveryProfile(
	ctx context.Context,
	cfg config,
	graphStore persistence.GraphStore,
	metadataStore persistence.MetadataStore,
	entityRegistry persistence.EntityRegistry,
	logger *logging.Logger,
) (*profile.ClusterProfile, error) {
	data, err := os.ReadFile(cfg.DiscoveryProfile)
	if err != nil {
		return nil, fmt.Errorf("read discovery profile: %w", err)
	}
	var prof profile.ClusterProfile
	if err := json.Unmarshal(data, &prof); err != nil {
		return nil, fmt.Errorf("unmarshal discovery profile: %w", err)
	}

	if err := metadataStore.PutDiscovery(ctx, prof); err != nil {
		return nil, fmt.Errorf("persist discovery profile: %w", err)
	}

	builder := ingest.NewK8sStructuralTopologyBuilder(cfg.ClusterName)
	ingestor := ingest.NewStructuralTopologyIngestor(builder, graphStore, entityRegistry, logger)
	if err := ingestor.IngestProfile(ctx, &prof); err != nil {
		return nil, fmt.Errorf("ingest structural topology: %w", err)
	}

	logger.Info("structural topology ingested from discovery profile",
		logging.String("profile_path", cfg.DiscoveryProfile),
		logging.String("cluster_name", prof.ClusterName),
	)
	return &prof, nil
}
