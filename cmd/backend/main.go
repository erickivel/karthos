package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/erickivel/oksg/pkg/adapters/otlp"
	"github.com/erickivel/oksg/pkg/adapters/storage"
	"github.com/erickivel/oksg/pkg/backend/ingest"
	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

const (
	defaultBackendGRPCListenAddr   = "0.0.0.0:4317"
	defaultBackendHTTPListenAddr   = "0.0.0.0:4318"
	defaultBackendHealthListenAddr = "0.0.0.0:13133"
	defaultBackendLogLevel         = "info"
	defaultStorageBackend          = "memory"
	defaultTopologyFlushInterval   = 60 * time.Second
)

type config struct {
	GRPCListenAddress   string
	HTTPListenAddress   string
	HealthListenAddress string
	LogLevel            string
	EnabledSignals      []string
	ClusterName         string
	DiscoveryProfile    string
	StorageBackend          string
	PostgresDSN             string
	VMURL                   string
	RawStoreEnabled         bool
	TopologyFlushInterval   time.Duration
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

	stores, err := storage.NewStores(ctx, storage.StorageConfig{
		Backend:     storage.StorageBackend(cfg.StorageBackend),
		PostgresDSN: cfg.PostgresDSN,
		VMURL:       cfg.VMURL,
		RawEnabled:  cfg.RawStoreEnabled,
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
	normalizer := ingest.NewOTLPNormalizer(resolver)
	traceTopoBuilder := ingest.NewTraceTopologyBuilder(resolver)
	metricTopoBuilder := ingest.NewMetricTopologyBuilder(cfg.ClusterName)

	if cfg.DiscoveryProfile != "" {
		if err := ingestDiscoveryProfile(ctx, cfg, stores.GraphStore, stores.MetadataStore, logger); err != nil {
			return err
		}
	}

	bufferedTopo := ingest.NewBufferedTopologyIngestor(
		map[schema.SignalType]ingest.TopologyBuilder{
			schema.SignalMetric: metricTopoBuilder,
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

	server, err := otlp.NewServer(otlp.Config{
		GRPCListenAddress:   cfg.GRPCListenAddress,
		HTTPListenAddress:   cfg.HTTPListenAddress,
		HealthListenAddress: cfg.HealthListenAddress,
		EnabledSignals:      cfg.EnabledSignals,
		MetricStore:         stores.MetricStore,
		GraphStore:          stores.GraphStore,
	}, ingest.NewEnvelopeBuilder(contextNow()), router, logger)
	if err != nil {
		return err
	}

	logger.Info("backend server configured",
		logging.String("grpc_listen_address", server.GRPCAddress()),
		logging.String("http_listen_address", server.HTTPAddress()),
		logging.String("health_listen_address", server.HealthAddress()),
		logging.Any("enabled_signals", cfg.EnabledSignals),
	)

	return server.Run(ctx)
}

func parseConfig(args []string) (config, error) {
	fs := flag.NewFlagSet("backend", flag.ContinueOnError)
	grpcListenAddress := fs.String("grpc-listen-address", envOrDefault("OKSG_BACKEND_GRPC_LISTEN_ADDRESS", defaultBackendGRPCListenAddr), "OTLP gRPC listen address")
	httpListenAddress := fs.String("http-listen-address", envOrDefault("OKSG_BACKEND_HTTP_LISTEN_ADDRESS", defaultBackendHTTPListenAddr), "OTLP HTTP listen address")
	healthListenAddress := fs.String("health-listen-address", envOrDefault("OKSG_BACKEND_HEALTH_LISTEN_ADDRESS", defaultBackendHealthListenAddr), "health listen address")
	logLevel := fs.String("log-level", envOrDefault("OKSG_LOG_LEVEL", defaultBackendLogLevel), "structured log level")
	enabledSignals := fs.String("enabled-signals", envOrDefault("OKSG_BACKEND_ENABLED_SIGNALS", "metric,trace"), "comma-separated enabled signals")
	clusterName := fs.String("cluster-name", envOrDefault("OKSG_CLUSTER_NAME", "default"), "cluster name for entity resolution")
	discoveryProfile := fs.String("discovery-profile", envOrDefault("OKSG_DISCOVERY_PROFILE", ""), "path to ClusterProfile JSON for structural topology")
	storageBackend := fs.String("storage-backend", envOrDefault("OKSG_STORAGE_BACKEND", defaultStorageBackend), "storage backend: memory or external")
	postgresDSN := fs.String("postgres-dsn", envOrDefault("OKSG_POSTGRES_DSN", ""), "PostgreSQL connection string")
	vmURL := fs.String("victoriametrics-url", envOrDefault("OKSG_VICTORIAMETRICS_URL", ""), "VictoriaMetrics base URL")
	rawStoreEnabled := fs.Bool("raw-store-enabled", envOrDefault("OKSG_RAW_STORE_ENABLED", "false") == "true", "enable raw envelope persistence")
	topologyFlushIntervalStr := fs.String("topology-flush-interval", envOrDefault("OKSG_TOPOLOGY_FLUSH_INTERVAL", defaultTopologyFlushInterval.String()), "topology snapshot flush interval")
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

	return config{
		GRPCListenAddress:     *grpcListenAddress,
		HTTPListenAddress:     *httpListenAddress,
		HealthListenAddress:   *healthListenAddress,
		LogLevel:              *logLevel,
		EnabledSignals:        signals,
		ClusterName:           *clusterName,
		DiscoveryProfile:      *discoveryProfile,
		StorageBackend:        *storageBackend,
		PostgresDSN:           *postgresDSN,
		VMURL:                 *vmURL,
		RawStoreEnabled:       *rawStoreEnabled,
		TopologyFlushInterval: topologyFlushInterval,
	}, nil
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

func ingestDiscoveryProfile(ctx context.Context, cfg config, graphStore persistence.GraphStore, metadataStore persistence.MetadataStore, logger *logging.Logger) error {
	data, err := os.ReadFile(cfg.DiscoveryProfile)
	if err != nil {
		return fmt.Errorf("read discovery profile: %w", err)
	}
	var prof profile.ClusterProfile
	if err := json.Unmarshal(data, &prof); err != nil {
		return fmt.Errorf("unmarshal discovery profile: %w", err)
	}

	if err := metadataStore.PutDiscovery(ctx, prof); err != nil {
		return fmt.Errorf("persist discovery profile: %w", err)
	}

	builder := ingest.NewK8sStructuralTopologyBuilder(cfg.ClusterName)
	ingestor := ingest.NewStructuralTopologyIngestor(builder, graphStore)
	if err := ingestor.IngestProfile(ctx, &prof); err != nil {
		return fmt.Errorf("ingest structural topology: %w", err)
	}

	logger.Info("structural topology ingested from discovery profile",
		logging.String("profile_path", cfg.DiscoveryProfile),
		logging.String("cluster_name", prof.ClusterName),
	)
	return nil
}
