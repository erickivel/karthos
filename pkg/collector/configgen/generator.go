package configgen

import (
	"fmt"
	"net"
	"strconv"

	"sigs.k8s.io/yaml"

	"github.com/erickivel/oksg/pkg/adapters/otel"
)

const (
	DefaultReleaseName         = "oksg-otel"
	DefaultChartPath           = "./deploy/helm/oksg-otel"
	DefaultLogLevel            = "info"
	DefaultLogFormat           = "json"
	DefaultCollectorRepository = "otel/opentelemetry-collector-contrib"
	DefaultCollectorTag        = "0.156.0"
	DefaultCollectorPullPolicy = "IfNotPresent"
	DefaultBackendRepository   = "oksg-backend"
	DefaultBackendTag          = "latest"
	DefaultBackendPullPolicy   = "IfNotPresent"
	DefaultBackendHealthPort   = 13133
)

type Generator struct {
	options GeneratorOptions
}

type GeneratorOptions struct {
	CollectorImage ImageValues
	BackendImage   ImageValues
	ReleaseName    string
	ChartPath      string
}

type Values struct {
	Global         GlobalValues  `json:"global" yaml:"global"`
	EnabledSignals []string      `json:"enabledSignals,omitempty" yaml:"enabledSignals,omitempty"`
	Agent          AgentValues   `json:"agent" yaml:"agent"`
	Gateway        GatewayValues `json:"gateway" yaml:"gateway"`
	Backend        BackendValues `json:"backend" yaml:"backend"`
	Secrets        SecretsValues `json:"secrets" yaml:"secrets"`
	Rollout        RolloutValues `json:"rollout" yaml:"rollout"`
	Namespace      NamespaceSpec `json:"namespace" yaml:"namespace"`
}

type GlobalValues struct {
	ClusterName string      `json:"clusterName" yaml:"clusterName"`
	Namespace   string      `json:"namespace" yaml:"namespace"`
	ReleaseName string      `json:"releaseName" yaml:"releaseName"`
	ChartPath   string      `json:"chartPath" yaml:"chartPath"`
	Image       ImageValues `json:"image" yaml:"image"`
	Logging     LoggingSpec `json:"logging" yaml:"logging"`
}

type ImageValues struct {
	Repository string `json:"repository" yaml:"repository"`
	Tag        string `json:"tag" yaml:"tag"`
	PullPolicy string `json:"pullPolicy" yaml:"pullPolicy"`
}

type LoggingSpec struct {
	Format string `json:"format" yaml:"format"`
	Level  string `json:"level" yaml:"level"`
}

type NamespaceSpec struct {
	Create bool   `json:"create" yaml:"create"`
	Name   string `json:"name" yaml:"name"`
}

type ComponentValues struct {
	Enabled       bool              `json:"enabled" yaml:"enabled"`
	Logging       LoggingSpec       `json:"logging" yaml:"logging"`
	FeatureGates  []string          `json:"featureGates,omitempty" yaml:"featureGates,omitempty"`
	RuntimeEnv    map[string]string `json:"runtimeEnv,omitempty" yaml:"runtimeEnv,omitempty"`
	CollectorYAML string            `json:"collectorConfig" yaml:"collectorConfig"`
}

type AgentValues struct {
	ComponentValues `json:",inline" yaml:",inline"`
	HostNetwork     bool              `json:"hostNetwork" yaml:"hostNetwork"`
	Resources       map[string]any    `json:"resources,omitempty" yaml:"resources,omitempty"`
	Tolerations     []map[string]any  `json:"tolerations,omitempty" yaml:"tolerations,omitempty"`
	NodeSelector    map[string]string `json:"nodeSelector,omitempty" yaml:"nodeSelector,omitempty"`
}

type GatewayValues struct {
	ComponentValues `json:",inline" yaml:",inline"`
	Replicas        int            `json:"replicas" yaml:"replicas"`
	Service         ServiceValues  `json:"service" yaml:"service"`
	Resources       map[string]any `json:"resources,omitempty" yaml:"resources,omitempty"`
}

type ServiceValues struct {
	Type     string `json:"type" yaml:"type"`
	Port     int    `json:"port" yaml:"port"`
	HTTPPort int    `json:"httpPort" yaml:"httpPort"`
}

type BackendValues struct {
	Enabled          bool                   `json:"enabled" yaml:"enabled"`
	GRPCEndpoint     string                 `json:"grpcEndpoint,omitempty" yaml:"grpcEndpoint,omitempty"`
	HTTPEndpoint     string                 `json:"httpEndpoint,omitempty" yaml:"httpEndpoint,omitempty"`
	Image            ImageValues            `json:"image" yaml:"image"`
	Service          BackendService         `json:"service" yaml:"service"`
	RuntimeEnv       map[string]string      `json:"runtimeEnv,omitempty" yaml:"runtimeEnv,omitempty"`
	DiscoveryProfile DiscoveryProfileValues `json:"discoveryProfile,omitempty" yaml:"discoveryProfile,omitempty"`
	Resources        map[string]any         `json:"resources,omitempty" yaml:"resources,omitempty"`
}

type DiscoveryProfileValues struct {
	Enabled       bool   `json:"enabled" yaml:"enabled"`
	ConfigMapName string `json:"configMapName,omitempty" yaml:"configMapName,omitempty"`
}

type BackendService struct {
	GRPCPort   int `json:"grpcPort" yaml:"grpcPort"`
	HTTPPort   int `json:"httpPort" yaml:"httpPort"`
	HealthPort int `json:"healthPort" yaml:"healthPort"`
}

type SecretsValues struct {
	ExistingSecretName string `json:"existingSecretName,omitempty" yaml:"existingSecretName,omitempty"`
}

type RolloutValues struct {
	UseConfigChecksum bool `json:"useConfigChecksum" yaml:"useConfigChecksum"`
}

func NewGenerator() *Generator {
	return &Generator{
		options: defaultGeneratorOptions(),
	}
}

func NewGeneratorWithOptions(options GeneratorOptions) *Generator {
	return &Generator{
		options: withDefaultGeneratorOptions(options),
	}
}

func (g *Generator) Generate(plan otel.Plan) ([]byte, error) {
	values, err := g.Build(plan)
	if err != nil {
		return nil, err
	}

	payload, err := yaml.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshal helm values: %w", err)
	}
	return payload, nil
}

func (g *Generator) Build(plan otel.Plan) (*Values, error) {
	agentConfig, err := yaml.Marshal(plan.AgentConfig)
	if err != nil {
		return nil, fmt.Errorf("marshal agent config: %w", err)
	}

	gatewayConfig, err := yaml.Marshal(plan.GatewayConfig)
	if err != nil {
		return nil, fmt.Errorf("marshal gateway config: %w", err)
	}

	logging := LoggingSpec{
		Format: DefaultLogFormat,
		Level:  DefaultLogLevel,
	}

	return &Values{
		Global: GlobalValues{
			ClusterName: plan.ClusterName,
			Namespace:   plan.Namespace,
			ReleaseName: g.options.ReleaseName,
			ChartPath:   g.options.ChartPath,
			Image:       g.options.CollectorImage,
			Logging:     logging,
		},
		EnabledSignals: plan.EnabledSignals,
		Namespace: NamespaceSpec{
			Create: true,
			Name:   plan.Namespace,
		},
		Agent: AgentValues{
			ComponentValues: ComponentValues{
				Enabled:       true,
				Logging:       logging,
				FeatureGates:  []string{},
				CollectorYAML: string(agentConfig),
			},
			HostNetwork: true,
		},
		Gateway: GatewayValues{
			ComponentValues: ComponentValues{
				Enabled:       true,
				Logging:       logging,
				FeatureGates:  []string{"-connector.spanmetrics.includeCollectorInstanceID"},
				CollectorYAML: string(gatewayConfig),
			},
			Replicas: 1,
			Service: ServiceValues{
				Type:     "ClusterIP",
				Port:     4317,
				HTTPPort: 4318,
			},
		},
		Backend: BackendValues{
			Enabled:      plan.Backend.Enabled,
			GRPCEndpoint: plan.Backend.GRPCEndpoint,
			HTTPEndpoint: plan.Backend.HTTPEndpoint,
			Image:        g.options.BackendImage,
			Service: BackendService{
				GRPCPort:   endpointPort(plan.Backend.GRPCEndpoint, 4317),
				HTTPPort:   endpointPort(plan.Backend.HTTPEndpoint, 4318),
				HealthPort: DefaultBackendHealthPort,
			},
			DiscoveryProfile: DiscoveryProfileValues{
				Enabled:       true,
				ConfigMapName: "oksg-discovery-profile",
			},
		},
		Secrets: SecretsValues{},
		Rollout: RolloutValues{
			UseConfigChecksum: true,
		},
	}, nil
}

func defaultGeneratorOptions() GeneratorOptions {
	return GeneratorOptions{
		CollectorImage: ImageValues{
			Repository: DefaultCollectorRepository,
			Tag:        DefaultCollectorTag,
			PullPolicy: DefaultCollectorPullPolicy,
		},
		BackendImage: ImageValues{
			Repository: DefaultBackendRepository,
			Tag:        DefaultBackendTag,
			PullPolicy: DefaultBackendPullPolicy,
		},
		ReleaseName: DefaultReleaseName,
		ChartPath:   DefaultChartPath,
	}
}

func withDefaultGeneratorOptions(options GeneratorOptions) GeneratorOptions {
	defaults := defaultGeneratorOptions()
	if options.CollectorImage.Repository == "" {
		options.CollectorImage.Repository = defaults.CollectorImage.Repository
	}
	if options.CollectorImage.Tag == "" {
		options.CollectorImage.Tag = defaults.CollectorImage.Tag
	}
	if options.CollectorImage.PullPolicy == "" {
		options.CollectorImage.PullPolicy = defaults.CollectorImage.PullPolicy
	}
	if options.BackendImage.Repository == "" {
		options.BackendImage.Repository = defaults.BackendImage.Repository
	}
	if options.BackendImage.Tag == "" {
		options.BackendImage.Tag = defaults.BackendImage.Tag
	}
	if options.BackendImage.PullPolicy == "" {
		options.BackendImage.PullPolicy = defaults.BackendImage.PullPolicy
	}
	if options.ReleaseName == "" {
		options.ReleaseName = defaults.ReleaseName
	}
	if options.ChartPath == "" {
		options.ChartPath = defaults.ChartPath
	}
	return options
}

func endpointPort(endpoint string, fallback int) int {
	_, port, err := net.SplitHostPort(endpoint)
	if err != nil {
		return fallback
	}
	value, err := strconv.Atoi(port)
	if err != nil {
		return fallback
	}
	return value
}
