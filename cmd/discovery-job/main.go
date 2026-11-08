package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strconv"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	k8sadapter "github.com/erickivel/karthos/pkg/adapters/kubernetes"
	"github.com/erickivel/karthos/pkg/adapters/otel"
	"github.com/erickivel/karthos/pkg/collector"
	"github.com/erickivel/karthos/pkg/collector/configgen"
	"github.com/erickivel/karthos/pkg/collector/deployer"
	"github.com/erickivel/karthos/pkg/core/logging"
	"github.com/erickivel/karthos/pkg/discovery/profile"
	"github.com/erickivel/karthos/pkg/sources"
	"github.com/erickivel/karthos/pkg/sources/contract"
)

type planner interface {
	Plan(ctx context.Context, profile *profile.ClusterProfile) (*deploymentPlan, error)
}

type generator interface {
	Generate(deployment otel.Deployment, capabilityProfile profile.ClusterProfile, sourcePlan contract.ResolvedSourcePlan) ([]byte, error)
}

type deploymentPlan struct {
	Deployment        otel.Deployment
	CapabilityProfile profile.ClusterProfile
	SourcePlan        contract.ResolvedSourcePlan
}

type commandPlanner struct {
	modules  sources.Registry
	composer *collector.Composer
	compiler *otel.Compiler
}

func newCommandPlanner(namespace string, collectorDebug bool) *commandPlanner {
	return &commandPlanner{
		modules:  sources.DefaultRegistry(),
		composer: collector.NewComposer(namespace),
		compiler: otel.NewCompilerWithOptions(otel.CompilerOptions{DebugEnabled: collectorDebug}),
	}
}

func (p *commandPlanner) Plan(_ context.Context, capabilityProfile *profile.ClusterProfile) (*deploymentPlan, error) {
	sourcePlan, err := p.modules.ResolveSourcePlan(capabilityProfile)
	if err != nil {
		return nil, err
	}
	collectionPlan, err := p.composer.Compose(capabilityProfile, sourcePlan)
	if err != nil {
		return nil, err
	}
	deployment, err := p.compiler.Compile(collectionPlan)
	if err != nil {
		return nil, err
	}
	return &deploymentPlan{Deployment: deployment, CapabilityProfile: *capabilityProfile, SourcePlan: sourcePlan}, nil
}

type deployerInterface interface {
	Apply(ctx context.Context, values []byte, options deployer.Options) error
	Template(ctx context.Context, values []byte, options deployer.Options) error
}

type dependencies struct {
	discover  func(context.Context) (*profile.ClusterProfile, error)
	planner   planner
	generator generator
	deployer  deployerInterface
}

func main() {
	if err := run(context.Background(), os.Args[1:], dependencies{}); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

const defaultClusterName = "default-cluster"
const defaultLogLevel = "info"

func run(ctx context.Context, args []string, deps dependencies) error {
	collectorDebugDefault, err := envBool("KARTHOS_COLLECTOR_DEBUG", false)
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("discovery-job", flag.ContinueOnError)
	mode := fs.String("mode", "discover", "mode to run: discover, plan, deploy")
	kubeconfig := fs.String("kubeconfig", "", "path to kubeconfig")
	clusterName := fs.String("cluster-name", defaultClusterName, "name of the cluster being discovered")
	profilePath := fs.String("capability-profile", "", "path to capability profile JSON")
	outputPath := fs.String("output", "", "output file path for discover capability profile")
	valuesOutputPath := fs.String("values-output", "", "output file path for generated Helm values")
	resolvedPlanOutputPath := fs.String("resolved-plan-output", "", "output path for resolved source plan JSON (defaults beside --values-output)")
	renderedOutputPath := fs.String("rendered-output", "", "directory for helm template output")
	namespace := fs.String("namespace", collector.DefaultNamespace, "namespace for generated collector resources")
	releaseName := fs.String("release-name", deployer.DefaultReleaseName, "helm release name")
	chartPath := fs.String("chart-path", deployer.DefaultChartPath, "path to the Helm chart")
	helmTimeout := fs.Duration("helm-timeout", deployer.DefaultTimeout, "timeout for helm operations")
	helmDryRun := fs.Bool("helm-dry-run", false, "run helm upgrade with --dry-run")
	logLevel := fs.String("log-level", envOrDefault("KARTHOS_LOG_LEVEL", defaultLogLevel), "structured log level: info, warn, error")
	targetNamespace := fs.String("target-namespace", "", "filter Prometheus discovery to this namespace (empty = all namespaces)")
	collectorImageRepository := fs.String("collector-image-repository", envOrDefault("KARTHOS_COLLECTOR_IMAGE_REPOSITORY", ""), "collector image repository override")
	collectorImageTag := fs.String("collector-image-tag", envOrDefault("KARTHOS_COLLECTOR_IMAGE_TAG", ""), "collector image tag override")
	collectorImagePullPolicy := fs.String("collector-image-pull-policy", envOrDefault("KARTHOS_COLLECTOR_IMAGE_PULL_POLICY", ""), "collector image pull policy override")
	backendImageRepository := fs.String("backend-image-repository", envOrDefault("KARTHOS_BACKEND_IMAGE_REPOSITORY", ""), "backend image repository override")
	backendImageTag := fs.String("backend-image-tag", envOrDefault("KARTHOS_BACKEND_IMAGE_TAG", ""), "backend image tag override")
	backendImagePullPolicy := fs.String("backend-image-pull-policy", envOrDefault("KARTHOS_BACKEND_IMAGE_PULL_POLICY", ""), "backend image pull policy override")
	frontendImageRepository := fs.String("frontend-image-repository", envOrDefault("KARTHOS_FRONTEND_IMAGE_REPOSITORY", ""), "frontend image repository override")
	frontendImageTag := fs.String("frontend-image-tag", envOrDefault("KARTHOS_FRONTEND_IMAGE_TAG", ""), "frontend image tag override")
	frontendImagePullPolicy := fs.String("frontend-image-pull-policy", envOrDefault("KARTHOS_FRONTEND_IMAGE_PULL_POLICY", ""), "frontend image pull policy override")
	storageBackend := fs.String("storage-backend", envOrDefault("KARTHOS_DEPLOY_STORAGE_BACKEND", ""), "storage backend override: memory or external")
	postgresqlMode := fs.String("postgresql-mode", envOrDefault("KARTHOS_DEPLOY_POSTGRESQL_MODE", ""), "external PostgreSQL mode: bundled or remote")
	postgresqlDSN := fs.String("postgresql-dsn", envOrDefault("KARTHOS_POSTGRES_DSN", ""), "remote PostgreSQL DSN override")
	victoriaMetricsMode := fs.String("victoriametrics-mode", envOrDefault("KARTHOS_DEPLOY_VICTORIAMETRICS_MODE", ""), "external VictoriaMetrics mode: bundled or remote")
	victoriaMetricsURL := fs.String("victoriametrics-url", envOrDefault("KARTHOS_VICTORIAMETRICS_URL", ""), "remote VictoriaMetrics URL override")
	collectorDebug := fs.Bool("collector-debug", collectorDebugDefault, "enable development-only Collector debug output")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if *mode != "discover" && *outputPath != "" {
		return fmt.Errorf("--output is only supported in discover mode; use --values-output for generated Helm values")
	}

	logger, err := logging.New("discovery-job", *logLevel)
	if err != nil {
		return err
	}

	activeNamespace := *namespace

	valuesPath := *valuesOutputPath
	resolvedPlanPath := coalesce(*resolvedPlanOutputPath, defaultResolvedPlanPath(valuesPath))
	deployOptions := deployer.Options{
		ReleaseName:  *releaseName,
		ChartPath:    *chartPath,
		Namespace:    activeNamespace,
		Timeout:      *helmTimeout,
		DryRun:       *helmDryRun,
		RenderedPath: *renderedOutputPath,
	}
	generatorOptions := configgen.GeneratorOptions{Runtime: configgen.RuntimeSettings{
		CollectorImage: configgen.ImageValues{Repository: *collectorImageRepository, Tag: *collectorImageTag, PullPolicy: *collectorImagePullPolicy},
		BackendImage:   configgen.ImageValues{Repository: *backendImageRepository, Tag: *backendImageTag, PullPolicy: *backendImagePullPolicy},
		FrontendImage:  configgen.ImageValues{Repository: *frontendImageRepository, Tag: *frontendImageTag, PullPolicy: *frontendImagePullPolicy},
		Storage: configgen.StorageSettings{
			Backend:         configgen.StorageBackend(*storageBackend),
			PostgreSQL:      configgen.StoreSettings{Mode: configgen.StoreMode(*postgresqlMode), Endpoint: *postgresqlDSN},
			VictoriaMetrics: configgen.StoreSettings{Mode: configgen.StoreMode(*victoriaMetricsMode), Endpoint: *victoriaMetricsURL},
		},
	}}

	logger.Info("starting command",
		logging.String("mode", *mode),
		logging.String("namespace", activeNamespace),
		logging.String("release_name", *releaseName),
	)

	switch *mode {
	case "discover":
		if deps.discover == nil {
			deps.discover = realDependenciesWithKubeconfig(*kubeconfig, *clusterName, *targetNamespace, activeNamespace).discover
		}
		clusterProfile, err := deps.discover(ctx)
		if err != nil {
			logger.Error("discovery failed", logging.Error(err), logging.String("cluster", *clusterName))
			return err
		}
		if err := writeJSON(*outputPath, clusterProfile); err != nil {
			logger.Error("write profile failed", logging.Error(err))
			return err
		}
		logger.Info("profile written",
			logging.String("cluster", *clusterName),
			logging.String("output_path", coalesce(*outputPath, "stdout")),
		)
		return nil
	case "plan":
		clusterProfile, err := readProfile(*profilePath)
		if err != nil {
			logger.Error("read profile failed", logging.Error(err), logging.String("profile_path", *profilePath))
			return err
		}
		if deps.planner == nil {
			deps.planner = newCommandPlanner(activeNamespace, *collectorDebug)
		}
		if deps.generator == nil {
			deps.generator = defaultGenerator(generatorOptions)
		}
		plan, err := deps.planner.Plan(ctx, clusterProfile)
		if err != nil {
			logger.Error("planning failed", logging.Error(err))
			return err
		}
		values, err := deps.generator.Generate(plan.Deployment, plan.CapabilityProfile, plan.SourcePlan)
		if err != nil {
			logger.Error("generate helm values failed", logging.Error(err))
			return err
		}
		if err := writeOutput(valuesPath, values); err != nil {
			logger.Error("write helm values failed", logging.Error(err))
			return err
		}
		if resolvedPlanPath != "" {
			if err := writeJSON(resolvedPlanPath, plan.SourcePlan); err != nil {
				return fmt.Errorf("write resolved source plan: %w", err)
			}
		}
		if *renderedOutputPath != "" {
			if deps.deployer == nil {
				deps.deployer = deployer.New(nil)
			}
			if err := deps.deployer.Template(ctx, values, deployOptions); err != nil {
				logger.Error("helm template failed", logging.Error(err))
				return err
			}
		}
		logger.Info("helm values generated",
			logging.String("profile_path", *profilePath),
			logging.String("output_path", coalesce(valuesPath, "stdout")),
			logging.String("cluster", string(clusterProfile.ClusterID)),
		)
		return nil
	case "deploy":
		clusterProfile, err := readProfile(*profilePath)
		if err != nil {
			logger.Error("read profile failed", logging.Error(err), logging.String("profile_path", *profilePath))
			return err
		}
		if deps.planner == nil {
			deps.planner = newCommandPlanner(activeNamespace, *collectorDebug)
		}
		if deps.generator == nil {
			deps.generator = defaultGenerator(generatorOptions)
		}
		if deps.deployer == nil {
			deps.deployer = deployer.New(nil)
		}
		plan, err := deps.planner.Plan(ctx, clusterProfile)
		if err != nil {
			logger.Error("planning failed", logging.Error(err))
			return err
		}
		values, err := deps.generator.Generate(plan.Deployment, plan.CapabilityProfile, plan.SourcePlan)
		if err != nil {
			logger.Error("generate helm values failed", logging.Error(err))
			return err
		}
		if valuesPath != "" {
			if err := writeOutput(valuesPath, values); err != nil {
				logger.Error("write helm values failed", logging.Error(err))
				return err
			}
		}
		if resolvedPlanPath != "" {
			if err := writeJSON(resolvedPlanPath, plan.SourcePlan); err != nil {
				return fmt.Errorf("write resolved source plan: %w", err)
			}
		}
		logger.Info("deploying helm release",
			logging.String("release_name", *releaseName),
			logging.String("chart_path", *chartPath),
			logging.String("namespace", activeNamespace),
			logging.String("cluster", string(clusterProfile.ClusterID)),
		)
		if err := deps.deployer.Apply(ctx, values, deployOptions); err != nil {
			logger.Error("helm deploy failed", logging.Error(err))
			return err
		}
		logger.Info("helm release deployed",
			logging.String("release_name", *releaseName),
			logging.String("namespace", activeNamespace),
		)
		return nil
	default:
		return fmt.Errorf("unsupported mode: %s", *mode)
	}
}

func realDependenciesWithKubeconfig(kubeconfig, clusterName, targetNamespace, installationNamespace string) dependencies {
	config, err := clientcmd.BuildConfigFromFlags("", kubeconfig)
	if err != nil {
		return dependencies{
			discover: func(context.Context) (*profile.ClusterProfile, error) {
				return nil, fmt.Errorf("build kubeconfig: %w", err)
			},
		}
	}

	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		return dependencies{
			discover: func(context.Context) (*profile.ClusterProfile, error) {
				return nil, fmt.Errorf("create kubernetes client: %w", err)
			},
		}
	}

	opts := []k8sadapter.DiscoveryOption{k8sadapter.WithExcludedNamespace(installationNamespace)}
	if targetNamespace != "" {
		opts = append(opts, k8sadapter.WithTargetNamespaces(targetNamespace))
	}

	adapter := k8sadapter.NewDiscoveryAdapter(clientset, clusterName, opts...)
	return dependencies{discover: adapter.Discover}
}

func defaultGenerator(options configgen.GeneratorOptions) generator {
	return configgen.NewGeneratorWithOptions(options)
}

func readProfile(path string) (*profile.ClusterProfile, error) {
	if path == "" {
		return nil, fmt.Errorf("profile path is required")
	}

	data, err := os.ReadFile(filepath.Clean(path)) //#nosec G304 -- path comes from trusted CLI flag --capability-profile
	if err != nil {
		return nil, fmt.Errorf("read profile: %w", err)
	}

	var clusterProfile profile.ClusterProfile
	if err := json.Unmarshal(data, &clusterProfile); err != nil {
		return nil, fmt.Errorf("decode profile: %w", err)
	}
	return &clusterProfile, nil
}

func writeJSON(path string, value any) error {
	payload, err := json.MarshalIndent(value, "", "  ")
	if err != nil {
		return fmt.Errorf("encode json: %w", err)
	}
	return writeOutput(path, payload)
}

func writeOutput(path string, payload []byte) error {
	if path == "" {
		_, err := os.Stdout.Write(append(payload, '\n'))
		return err
	}

	return os.WriteFile(filepath.Clean(path), append(payload, '\n'), 0o600) //#nosec G304 -- path from trusted CLI flag
}

func coalesce(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}

func defaultResolvedPlanPath(valuesPath string) string {
	if valuesPath == "" {
		return ""
	}
	return valuesPath + ".source-plan.json"
}

func envOrDefault(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

func envBool(key string, fallback bool) (bool, error) {
	value, found := os.LookupEnv(key)
	if !found || value == "" {
		return fallback, nil
	}
	parsed, err := strconv.ParseBool(value)
	if err != nil {
		return false, fmt.Errorf("%s must be a boolean: %w", key, err)
	}
	return parsed, nil
}
