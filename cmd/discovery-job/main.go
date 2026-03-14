package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"net/http"
	"os"
	"time"

	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"

	k8sadapter "github.com/erickivel/oksg/pkg/adapters/kubernetes"
	"github.com/erickivel/oksg/pkg/adapters/otel"
	"github.com/erickivel/oksg/pkg/collector/configgen"
	"github.com/erickivel/oksg/pkg/collector/deployer"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

type planner interface {
	Plan(ctx context.Context, profile *profile.ClusterProfile) (*otel.Plan, error)
}

type generator interface {
	Generate(plan otel.Plan) ([]byte, error)
}

type deployerInterface interface {
	Apply(ctx context.Context, manifests []byte) error
}

type deployFunc func(ctx context.Context, manifests []byte) error

func (f deployFunc) Apply(ctx context.Context, manifests []byte) error {
	return f(ctx, manifests)
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

func run(ctx context.Context, args []string, deps dependencies) error {
	fs := flag.NewFlagSet("discovery-job", flag.ContinueOnError)
	mode := fs.String("mode", "discover", "mode to run: discover, plan, deploy")
	kubeconfig := fs.String("kubeconfig", "", "path to kubeconfig")
	clusterName := fs.String("cluster-name", defaultClusterName, "name of the cluster being discovered")
	profilePath := fs.String("profile", "", "path to discovery profile JSON")
	outputPath := fs.String("output", "", "output file path")
	namespace := fs.String("namespace", otel.DefaultNamespace, "namespace for generated collector resources")
	targetNamespace := fs.String("target-namespace", "", "filter Prometheus discovery to this namespace (empty = all namespaces)")
	if err := fs.Parse(args); err != nil {
		return err
	}

	switch *mode {
	case "discover":
		if deps.discover == nil {
			deps.discover = realDependenciesWithKubeconfig(*kubeconfig, *clusterName, *namespace, *targetNamespace).discover
		}
		clusterProfile, err := deps.discover(ctx)
		if err != nil {
			return err
		}
		return writeJSON(*outputPath, clusterProfile)
	case "plan":
		clusterProfile, err := readProfile(*profilePath)
		if err != nil {
			return err
		}
		if deps.planner == nil {
			deps.planner = otel.NewSourcePlanner(*namespace)
		}
		if deps.generator == nil {
			deps.generator = defaultGenerator()
		}
		plan, err := deps.planner.Plan(ctx, clusterProfile)
		if err != nil {
			return err
		}
		manifests, err := deps.generator.Generate(*plan)
		if err != nil {
			return err
		}
		return writeOutput(*outputPath, manifests)
	case "deploy":
		clusterProfile, err := readProfile(*profilePath)
		if err != nil {
			return err
		}
		if deps.planner == nil {
			deps.planner = otel.NewSourcePlanner(*namespace)
		}
		if deps.generator == nil {
			deps.generator = defaultGenerator()
		}
		if deps.deployer == nil {
			deps.deployer = deployer.New(nil)
		}
		plan, err := deps.planner.Plan(ctx, clusterProfile)
		if err != nil {
			return err
		}
		manifests, err := deps.generator.Generate(*plan)
		if err != nil {
			return err
		}
		if err := deps.deployer.Apply(ctx, manifests); err != nil {
			return err
		}
		if *outputPath != "" {
			return writeOutput(*outputPath, manifests)
		}
		return nil
	default:
		return fmt.Errorf("unsupported mode: %s", *mode)
	}
}

func realDependenciesWithKubeconfig(kubeconfig, clusterName, namespace, targetNamespace string) dependencies {
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

	adapter := k8sadapter.NewDiscoveryAdapter(clientset, clusterName, httpProbe, opts...)
	return dependencies{
		discover:  adapter.Discover,
		planner:   otel.NewSourcePlanner(namespace),
		generator: defaultGenerator(),
		deployer:  deployer.New(nil),
	}
}

func defaultGenerator() generator {
	return configgen.NewGenerator()
}

func httpProbe(ctx context.Context, url string) bool {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return false
	}

	client := &http.Client{Timeout: 3 * time.Second}
	response, err := client.Do(request)
	if err != nil {
		return false
	}
	defer response.Body.Close()

	return response.StatusCode >= 200 && response.StatusCode < 400
}

func readProfile(path string) (*profile.ClusterProfile, error) {
	if path == "" {
		return nil, fmt.Errorf("profile path is required")
	}

	data, err := os.ReadFile(path)
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

	return os.WriteFile(path, append(payload, '\n'), 0o644)
}
