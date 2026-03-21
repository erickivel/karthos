package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/erickivel/oksg/pkg/adapters/otlp"
	"github.com/erickivel/oksg/pkg/backend/ingest"
	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/logging"
)

const (
	defaultBackendGRPCListenAddr   = "0.0.0.0:4317"
	defaultBackendHTTPListenAddr   = "0.0.0.0:4318"
	defaultBackendHealthListenAddr = "0.0.0.0:13133"
	defaultBackendLogLevel         = "info"
)

type config struct {
	GRPCListenAddress   string
	HTTPListenAddress   string
	HealthListenAddress string
	LogLevel            string
	EnabledSignals      []string
	ClusterName         string
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

	resolver := ingest.NewOTLPEntityResolver(cfg.ClusterName)
	normalizer := ingest.NewOTLPNormalizer(resolver)
	topoBuilder := ingest.NewTraceTopologyBuilder(resolver)
	metricStore := persistence.NewInMemoryMetricStore()
	graphStore := persistence.NewInMemoryGraphStore()

	router := ingest.NewSignalRouter(
		ingest.NewNormalizingIngestor(normalizer, metricStore, logger),
		ingest.NewTopologyIngestor(topoBuilder, graphStore),
	)

	logger.Info("normalization pipeline configured",
		logging.String("cluster_name", cfg.ClusterName),
	)

	server, err := otlp.NewServer(otlp.Config{
		GRPCListenAddress:   cfg.GRPCListenAddress,
		HTTPListenAddress:   cfg.HTTPListenAddress,
		HealthListenAddress: cfg.HealthListenAddress,
		EnabledSignals:      cfg.EnabledSignals,
		MetricStore:         metricStore,
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
	if err := fs.Parse(args); err != nil {
		return config{}, err
	}

	signals, err := parseSignals(*enabledSignals)
	if err != nil {
		return config{}, err
	}

	return config{
		GRPCListenAddress:   *grpcListenAddress,
		HTTPListenAddress:   *httpListenAddress,
		HealthListenAddress: *healthListenAddress,
		LogLevel:            *logLevel,
		EnabledSignals:      signals,
		ClusterName:         *clusterName,
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
