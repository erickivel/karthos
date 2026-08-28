package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	k8sadapter "github.com/erickivel/oksg/pkg/adapters/kubernetes"
	"github.com/erickivel/oksg/pkg/adapters/otel"
	"github.com/erickivel/oksg/pkg/collector"
	"github.com/erickivel/oksg/pkg/collector/configgen"
	"github.com/erickivel/oksg/pkg/collector/deployer"
	"github.com/erickivel/oksg/pkg/core/logging"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources"
	"github.com/erickivel/oksg/pkg/sources/contract"
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

func newCommandPlanner(namespace string) *commandPlanner {
	return &commandPlanner{modules: sources.DefaultRegistry(), composer: collector.NewComposer(namespace), compiler: otel.NewCompiler()}
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
const defaultReleaseName = "oksg-otel"
const defaultLogLevel = "info"

func run(ctx context.Context, args []string, deps dependencies) error {
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
	helmNamespace := fs.String("helm-namespace", "", "namespace for Helm release resources (defaults to --namespace)")
	releaseName := fs.String("release-name", defaultReleaseName, "helm release name")
	chartPath := fs.String("chart-path", configgen.DefaultChartPath, "path to the Helm chart")
	helmTimeout := fs.Duration("helm-timeout", deployer.DefaultTimeout, "timeout for helm operations")
	helmDryRun := fs.Bool("helm-dry-run", false, "run helm upgrade with --dry-run")
	logLevel := fs.String("log-level", envOrDefault("OKSG_LOG_LEVEL", defaultLogLevel), "structured log level: info, warn, error")
	targetNamespace := fs.String("target-namespace", "", "filter Prometheus discovery to this namespace (empty = all namespaces)")
	collectorImageRepository := fs.String("collector-image-repository", envOrDefault("OKSG_COLLECTOR_IMAGE_REPOSITORY", configgen.DefaultCollectorRepository), "collector image repository")
	collectorImageTag := fs.String("collector-image-tag", envOrDefault("OKSG_COLLECTOR_IMAGE_TAG", configgen.DefaultCollectorTag), "collector image tag")
	collectorImagePullPolicy := fs.String("collector-image-pull-policy", envOrDefault("OKSG_COLLECTOR_IMAGE_PULL_POLICY", configgen.DefaultCollectorPullPolicy), "collector image pull policy")
	backendImageRepository := fs.String("backend-image-repository", envOrDefault("OKSG_BACKEND_IMAGE_REPOSITORY", configgen.DefaultBackendRepository), "backend image repository")
	backendImageTag := fs.String("backend-image-tag", envOrDefault("OKSG_BACKEND_IMAGE_TAG", configgen.DefaultBackendTag), "backend image tag")
	backendImagePullPolicy := fs.String("backend-image-pull-policy", envOrDefault("OKSG_BACKEND_IMAGE_PULL_POLICY", configgen.DefaultBackendPullPolicy), "backend image pull policy")
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
	if *helmNamespace != "" {
		activeNamespace = *helmNamespace
	}

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
	generatorOptions := configgen.GeneratorOptions{
		CollectorImage: configgen.ImageValues{
			Repository: *collectorImageRepository,
			Tag:        *collectorImageTag,
			PullPolicy: *collectorImagePullPolicy,
		},
		BackendImage: configgen.ImageValues{
			Repository: *backendImageRepository,
			Tag:        *backendImageTag,
			PullPolicy: *backendImagePullPolicy,
		},
		ReleaseName: *releaseName,
		ChartPath:   *chartPath,
	}

	logger.Info("starting command",
		logging.String("mode", *mode),
		logging.String("namespace", activeNamespace),
		logging.String("release_name", *releaseName),
	)

	switch *mode {
	case "discover":
		if deps.discover == nil {
			deps.discover = realDependenciesWithKubeconfig(*kubeconfig, *clusterName, *targetNamespace).discover
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
			deps.planner = newCommandPlanner(activeNamespace)
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
			deps.planner = newCommandPlanner(activeNamespace)
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

func realDependenciesWithKubeconfig(kubeconfig, clusterName, targetNamespace string) dependencies {
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

	var opts []k8sadapter.DiscoveryOption
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
