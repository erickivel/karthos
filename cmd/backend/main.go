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
	kubernetesadapter "github.com/erickivel/oksg/pkg/adapters/kubernetes"
	"github.com/erickivel/oksg/pkg/adapters/otlp"
	"github.com/erickivel/oksg/pkg/adapters/storage"
	"github.com/erickivel/oksg/pkg/backend/ingest"
	"github.com/erickivel/oksg/pkg/backend/persistence"
	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources"
	"github.com/erickivel/oksg/pkg/sources/contract"
	"github.com/erickivel/oksg/pkg/sources/structure"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
)

const (
	defaultBackendGRPCListenAddr   = "0.0.0.0:4317"
	defaultBackendHealthListenAddr = "0.0.0.0:13133"
	defaultBackendAPIListenAddr    = "0.0.0.0:8080"
	defaultBackendLogLevel         = "info"
	defaultStorageBackend          = "memory"
	defaultTopologyFlushInterval   = 60 * time.Second
	defaultStatusStaleSeconds      = 300
	defaultNodeTTL                 = 24 * time.Hour
	// defaultStructureRefreshInterval refreshes the live Kubernetes API
	// topology. It must remain strictly below node TTL so structural-only
	// entities (workloads, ingress, configmaps, secrets, PVCs) never age out.
	defaultStructureRefreshInterval = 10 * time.Minute
)

type config struct {
	GRPCListenAddress     string
	HealthListenAddress   string
	APIListenAddress      string
	LogLevel              string
	EnabledSignals        []string
	ClusterName           string
	CapabilityProfile     string
	SourcePlan            string
	StorageBackend        string
	PostgresDSN           string
	VMURL                 string
	TopologyFlushInterval time.Duration
	StatusStaleSeconds    int
	NodeTTL               time.Duration
	PodLifecycleRequired  bool
	// StructureRefreshInterval directly refreshes live Kubernetes structure.
	// It must stay positive and, when node TTL is enabled, strictly less than it.
	StructureRefreshInterval time.Duration
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

	capabilityProfile, sourcePlan, err := loadSourceContract(cfg)
	if err != nil {
		return err
	}
	if err := stores.MetadataStore.PutDiscovery(ctx, *capabilityProfile); err != nil {
		return fmt.Errorf("persist capability profile: %w", err)
	}

	client, err := newInClusterClient()
	if err != nil {
		return err
	}
	structural := structure.New(client, cfg.ClusterName, stores.GraphStore, stores.EntityRegistry)
	if err := structural.Refresh(ctx); err != nil {
		return fmt.Errorf("refresh initial Kubernetes structure: %w", err)
	}
	go runStructureRefreshLoop(ctx, cfg.StructureRefreshInterval, structural, logger)

	resolver := ingest.NewOTLPEntityResolverWithRegistry(cfg.ClusterName, stores.EntityRegistry)
	modules := sources.DefaultRegistry()
	dialect := metrics.NewSourceDialectRegistry(modules.Dialects())
	assignment := sourcePlan.MetricAssignments
	logger.Info("mounted source plan loaded", logging.Any("metric_count", len(assignment)))

	sourcePolicy := ingest.NewMetricSourcePolicy(modules.MetricIDs(), assignment)
	normalizer := ingest.NewOTLPNormalizer(resolver, dialect, ingest.WithSourcePolicy(sourcePolicy))
	traceTopoBuilder := ingest.NewTraceTopologyBuilder(resolver)
	podLifecycle, err := startPodLifecycle(ctx, cfg, stores.GraphStore, logger, client)
	if err != nil {
		return err
	}
	metricTopoBuilder := ingest.NewMetricTopologyBuilder(cfg.ClusterName)
	if podLifecycle != nil {
		metricTopoBuilder = ingest.NewMetricTopologyBuilderWithLifecycle(cfg.ClusterName, podLifecycle)
	}
	metricCallsTopoBuilder := ingest.NewMetricCallsTopologyBuilder(dialect, cfg.ClusterName, sourcePolicy)

	bufferedTopo := ingest.NewBufferedTopologyIngestor(
		map[schema.SignalType]ingest.TopologyBuilder{
			schema.SignalMetric: ingest.NewCompositeTopologyBuilder(metricTopoBuilder, metricCallsTopoBuilder),
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
		HealthListenAddress: cfg.HealthListenAddress,
		EnabledSignals:      cfg.EnabledSignals,
	}, ingest.NewEnvelopeBuilder(contextNow), router, logger)
	if err != nil {
		return err
	}

	logger.Info("backend server configured",
		logging.String("grpc_listen_address", server.GRPCAddress()),
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
	healthListenAddress := fs.String("health-listen-address", envOrDefault("OKSG_BACKEND_HEALTH_LISTEN_ADDRESS", defaultBackendHealthListenAddr), "health listen address")
	apiListenAddress := fs.String("api-listen-address", envOrDefault("OKSG_BACKEND_API_LISTEN_ADDRESS", defaultBackendAPIListenAddr), "REST API listen address")
	logLevel := fs.String("log-level", envOrDefault("OKSG_LOG_LEVEL", defaultBackendLogLevel), "structured log level")
	enabledSignals := fs.String("enabled-signals", envOrDefault("OKSG_BACKEND_ENABLED_SIGNALS", "metric,trace"), "comma-separated enabled signals")
	clusterName := fs.String("cluster-name", envOrDefault("OKSG_CLUSTER_NAME", "default"), "cluster name for entity resolution")
	capabilityProfile := fs.String("capability-profile", envOrDefault("OKSG_CAPABILITY_PROFILE", ""), "path to mounted capability profile JSON")
	sourcePlan := fs.String("source-plan", envOrDefault("OKSG_SOURCE_PLAN", ""), "path to mounted resolved source plan JSON")
	storageBackend := fs.String("storage-backend", envOrDefault("OKSG_STORAGE_BACKEND", defaultStorageBackend), "storage backend: memory or external")
	postgresDSN := fs.String("postgres-dsn", envOrDefault("OKSG_POSTGRES_DSN", ""), "PostgreSQL connection string")
	vmURL := fs.String("victoriametrics-url", envOrDefault("OKSG_VICTORIAMETRICS_URL", ""), "VictoriaMetrics base URL")
	topologyFlushIntervalStr := fs.String("topology-flush-interval", envOrDefault("OKSG_TOPOLOGY_FLUSH_INTERVAL", defaultTopologyFlushInterval.String()), "topology snapshot flush interval")
	statusStaleSeconds := fs.Int("status-stale-seconds", parseIntOrDefault(envOrDefault("OKSG_BACKEND_STATUS_STALE_SECONDS", ""), defaultStatusStaleSeconds), "snapshot staleness threshold in seconds for status endpoint")
	nodeTTLStr := fs.String("node-ttl", envOrDefault("OKSG_NODE_TTL", defaultNodeTTL.String()), "topology node TTL; nodes absent from snapshots for longer than this age out (0 disables)")
	podLifecycleRequired := fs.Bool("pod-lifecycle-required", envOrDefault("OKSG_POD_LIFECYCLE_REQUIRED", "false") == "true", "require in-cluster pod lifecycle validation for metric runs_on edges")
	structureRefreshStr := fs.String("structure-refresh-interval", envOrDefault("OKSG_STRUCTURE_REFRESH_INTERVAL", defaultStructureRefreshInterval.String()), "interval to refresh live Kubernetes structural topology; must be positive and strictly less than node TTL")
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

	structureRefresh, err := time.ParseDuration(*structureRefreshStr)
	if err != nil {
		return config{}, fmt.Errorf("parse structure refresh interval: %w", err)
	}
	if err := validateStructureRefreshInterval(structureRefresh, nodeTTL); err != nil {
		return config{}, err
	}

	return config{
		GRPCListenAddress:        *grpcListenAddress,
		HealthListenAddress:      *healthListenAddress,
		APIListenAddress:         *apiListenAddress,
		LogLevel:                 *logLevel,
		EnabledSignals:           signals,
		ClusterName:              *clusterName,
		CapabilityProfile:        *capabilityProfile,
		SourcePlan:               *sourcePlan,
		StorageBackend:           *storageBackend,
		PostgresDSN:              *postgresDSN,
		VMURL:                    *vmURL,
		TopologyFlushInterval:    topologyFlushInterval,
		StatusStaleSeconds:       *statusStaleSeconds,
		NodeTTL:                  nodeTTL,
		PodLifecycleRequired:     *podLifecycleRequired,
		StructureRefreshInterval: structureRefresh,
	}, nil
}

func validateStructureRefreshInterval(interval, nodeTTL time.Duration) error {
	if interval <= 0 {
		return fmt.Errorf("structure refresh interval must be greater than zero")
	}
	if nodeTTL > 0 && interval >= nodeTTL {
		return fmt.Errorf("structure refresh interval must be strictly less than node TTL")
	}
	return nil
}

func newInClusterClient() (kubernetes.Interface, error) {
	kubeConfig, err := rest.InClusterConfig()
	if err != nil {
		return nil, fmt.Errorf("build in-cluster Kubernetes configuration for structural refresh: %w", err)
	}
	client, err := kubernetes.NewForConfig(kubeConfig)
	if err != nil {
		return nil, fmt.Errorf("create Kubernetes client for structural refresh: %w", err)
	}
	return client, nil
}

func startPodLifecycle(ctx context.Context, cfg config, graphStore persistence.GraphStore, logger *logging.Logger, client kubernetes.Interface) (ingest.PodLifecycle, error) {
	if !cfg.PodLifecycleRequired {
		return nil, nil
	}

	if client == nil {
		return nil, fmt.Errorf("required pod lifecycle validation needs an in-cluster Kubernetes client")
	}
	retirer, ok := graphStore.(persistence.RunsOnRetirer)
	if !ok {
		return nil, fmt.Errorf("required pod lifecycle validation needs a graph store that retires runs_on edges")
	}
	reconciler, ok := graphStore.(persistence.RunsOnReconciler)
	if !ok {
		return nil, fmt.Errorf("required pod lifecycle validation needs a graph store that reconciles runs_on edges")
	}
	terminalMarker, ok := graphStore.(persistence.RunsOnTerminalMarker)
	if !ok {
		return nil, fmt.Errorf("required pod lifecycle validation needs a graph store that records terminal runs_on deadlines")
	}

	tracker := kubernetesadapter.NewPodLifecycleTracker(client,
		kubernetesadapter.WithPodLifecycleStateObserver(func(state kubernetesadapter.PodLifecycleState) {
			fields := []logging.Field{
				logging.String("reason", state.Reason),
				logging.Any("active_pod_count", state.ActivePodCount),
			}
			if state.Ready {
				logger.Info("pod lifecycle validation synchronized", fields...)
				return
			}
			logger.Warn("pod lifecycle validation unavailable; runs_on refreshes are fail-closed", fields...)
		}),
		kubernetesadapter.WithPodRunsOnTerminalObserver(func(terminalCtx context.Context, grace kubernetesadapter.PodRunsOnTerminalGrace) (kubernetesadapter.PodRunsOnTerminalState, error) {
			state, err := terminalMarker.MarkRunsOnTerminal(terminalCtx, persistence.RunsOnTerminalGrace{
				ClusterName: cfg.ClusterName,
				Namespace:   grace.Namespace,
				PodName:     grace.PodName,
				PodUID:      grace.PodUID,
				NodeName:    grace.NodeName,
				PodID:       entities.ComputeEntityID(entities.KindPod, cfg.ClusterName, grace.Namespace, grace.PodName),
				NodeID:      entities.ComputeEntityID(entities.KindNode, cfg.ClusterName, grace.NodeName),
				ObservedAt:  grace.ObservedAt,
				Deadline:    grace.Deadline,
				Confirmed:   grace.Confirmed,
			})
			if state == persistence.RunsOnTerminalRetired {
				return kubernetesadapter.PodRunsOnTerminalRetired, err
			}
			return kubernetesadapter.PodRunsOnTerminalPending, err
		}),
		kubernetesadapter.WithPodRunsOnReconciliationObserver(func(reconciliationCtx context.Context, observed []kubernetesadapter.PodRunsOnAssignment) ([]kubernetesadapter.PodRunsOnTerminalGrace, error) {
			active := make([]persistence.RunsOnAssignment, 0, len(observed))
			for _, assignment := range observed {
				active = append(active, persistence.RunsOnAssignment{
					PodID:  entities.ComputeEntityID(entities.KindPod, cfg.ClusterName, assignment.Namespace, assignment.PodName),
					NodeID: entities.ComputeEntityID(entities.KindNode, cfg.ClusterName, assignment.NodeName),
					PodUID: assignment.PodUID,
				})
			}
			persisted, err := reconciler.ReconcileRunsOn(reconciliationCtx, cfg.ClusterName, active, time.Now().UTC())
			if err != nil {
				return nil, fmt.Errorf("reconcile persisted runs_on assignments: %w", err)
			}
			grace := make([]kubernetesadapter.PodRunsOnTerminalGrace, 0, len(persisted))
			for _, item := range persisted {
				grace = append(grace, kubernetesadapter.PodRunsOnTerminalGrace{
					Namespace:  item.Namespace,
					PodName:    item.PodName,
					PodUID:     item.PodUID,
					NodeName:   item.NodeName,
					ObservedAt: item.ObservedAt,
					Deadline:   item.Deadline,
					Confirmed:  item.Confirmed,
				})
			}
			return grace, nil
		}),
		kubernetesadapter.WithPodRunsOnRetirementObserver(func(retirementCtx context.Context, retirement kubernetesadapter.PodRunsOnRetirement) error {
			podID := entities.ComputeEntityID(entities.KindPod, cfg.ClusterName, retirement.Namespace, retirement.PodName)
			nodeID := entities.ComputeEntityID(entities.KindNode, cfg.ClusterName, retirement.NodeName)
			if err := retirer.RetireRunsOn(retirementCtx, podID, nodeID, retirement.PodUID, retirement.RetiredAt); err != nil {
				logger.Error("retire terminal pod runs_on edge failed",
					logging.String("namespace", retirement.Namespace),
					logging.String("pod", retirement.PodName),
					logging.String("pod_uid", retirement.PodUID),
					logging.String("node", retirement.NodeName),
					logging.Error(err),
				)
				return err
			}
			logger.Info("retired terminal pod runs_on edge",
				logging.String("namespace", retirement.Namespace),
				logging.String("pod", retirement.PodName),
				logging.String("pod_uid", retirement.PodUID),
				logging.String("node", retirement.NodeName),
				logging.String("retired_at", retirement.RetiredAt.Format(time.RFC3339Nano)),
			)
			return nil
		}),
	)
	if err := tracker.Start(ctx); err != nil {
		return nil, fmt.Errorf("start required pod lifecycle validation: %w", err)
	}
	return tracker, nil
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

type structuralRefresher interface {
	Refresh(context.Context) error
}

func runStructureRefreshLoop(ctx context.Context, interval time.Duration, refresher structuralRefresher, logger *logging.Logger) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	logger.Info("Kubernetes structure refresh loop started", logging.String("interval", interval.String()))
	for {
		select {
		case <-ctx.Done():
			logger.Info("Kubernetes structure refresh loop stopping")
			return
		case <-ticker.C:
			if err := refresher.Refresh(ctx); err != nil {
				logger.Warn("Kubernetes structure refresh failed", logging.Error(err))
			}
		}
	}
}

func loadSourceContract(cfg config) (*profile.ClusterProfile, contract.ResolvedSourcePlan, error) {
	if cfg.CapabilityProfile == "" {
		return nil, contract.ResolvedSourcePlan{}, fmt.Errorf("capability profile path is required")
	}
	if cfg.SourcePlan == "" {
		return nil, contract.ResolvedSourcePlan{}, fmt.Errorf("source plan path is required")
	}
	profileData, err := os.ReadFile(cfg.CapabilityProfile)
	if err != nil {
		return nil, contract.ResolvedSourcePlan{}, fmt.Errorf("read capability profile: %w", err)
	}
	var capabilityProfile profile.ClusterProfile
	if err := json.Unmarshal(profileData, &capabilityProfile); err != nil {
		return nil, contract.ResolvedSourcePlan{}, fmt.Errorf("decode capability profile: %w", err)
	}
	modules := sources.DefaultRegistry()
	if err := capabilityProfile.Validate(modules.IDs()); err != nil {
		return nil, contract.ResolvedSourcePlan{}, fmt.Errorf("validate mounted capability profile: %w", err)
	}
	planData, err := os.ReadFile(cfg.SourcePlan)
	if err != nil {
		return nil, contract.ResolvedSourcePlan{}, fmt.Errorf("read source plan: %w", err)
	}
	var sourcePlan contract.ResolvedSourcePlan
	if err := json.Unmarshal(planData, &sourcePlan); err != nil {
		return nil, contract.ResolvedSourcePlan{}, fmt.Errorf("decode source plan: %w", err)
	}
	fingerprint, err := capabilityProfile.Fingerprint()
	if err != nil {
		return nil, contract.ResolvedSourcePlan{}, fmt.Errorf("fingerprint capability profile: %w", err)
	}
	if sourcePlan.ProfileFingerprint == "" || sourcePlan.ProfileFingerprint != fingerprint {
		return nil, contract.ResolvedSourcePlan{}, fmt.Errorf("source plan profile fingerprint does not match mounted capability profile")
	}
	if err := sourcePlan.Validate(&capabilityProfile, modules.ContractInventory()); err != nil {
		return nil, contract.ResolvedSourcePlan{}, fmt.Errorf("validate mounted source plan: %w", err)
	}
	return &capabilityProfile, sourcePlan, nil
}
