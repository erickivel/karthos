package configgen

import (
	"fmt"

	"sigs.k8s.io/yaml"

	"github.com/erickivel/oksg/pkg/adapters/otel"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources/contract"
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
	DefaultBackendGRPCPort     = 4317
	DefaultBackendHealthPort   = 13133
	DefaultBackendAPIPort      = 8080
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
	Global         GlobalValues         `json:"global" yaml:"global"`
	EnabledSignals []string             `json:"enabledSignals,omitempty" yaml:"enabledSignals,omitempty"`
	Agent          AgentValues          `json:"agent" yaml:"agent"`
	Gateway        GatewayValues        `json:"gateway" yaml:"gateway"`
	Backend        BackendValues        `json:"backend" yaml:"backend"`
	CollectorDebug CollectorDebugValues `json:"collectorDebug" yaml:"collectorDebug"`
	Storage        StorageValues        `json:"storage" yaml:"storage"`
	Secrets        SecretsValues        `json:"secrets" yaml:"secrets"`
	Rollout        RolloutValues        `json:"rollout" yaml:"rollout"`
	Namespace      NamespaceSpec        `json:"namespace" yaml:"namespace"`
}

// CollectorDebugValues describes an OTel-owned debug exporter. Helm may attach
// it to generated pipelines for transient development diagnostics only.
type CollectorDebugValues struct {
	Enabled        bool           `json:"enabled" yaml:"enabled"`
	ExporterID     string         `json:"exporterID" yaml:"exporterID"`
	ExporterConfig map[string]any `json:"exporterConfig" yaml:"exporterConfig"`
}

type StorageValues struct {
	Backend         string                       `json:"backend" yaml:"backend"`
	PostgreSQL      StoragePostgreSQLValues      `json:"postgresql" yaml:"postgresql"`
	VictoriaMetrics StorageVictoriaMetricsValues `json:"victoriametrics" yaml:"victoriametrics"`
}

type StoragePostgreSQLValues struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	DSN     string `json:"dsn" yaml:"dsn"`
}

type StorageVictoriaMetricsValues struct {
	Enabled bool   `json:"enabled" yaml:"enabled"`
	URL     string `json:"url" yaml:"url"`
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
	Enabled        bool                 `json:"enabled" yaml:"enabled"`
	GRPCPort       int                  `json:"grpcPort" yaml:"grpcPort"`
	Image          ImageValues          `json:"image" yaml:"image"`
	Service        BackendService       `json:"service" yaml:"service"`
	RuntimeEnv     map[string]string    `json:"runtimeEnv,omitempty" yaml:"runtimeEnv,omitempty"`
	SourceContract SourceContractValues `json:"sourceContract,omitempty" yaml:"sourceContract,omitempty"`
	Resources      map[string]any       `json:"resources,omitempty" yaml:"resources,omitempty"`
}

type SourceContractValues struct {
	CapabilityProfile profile.ClusterProfile      `json:"capabilityProfile" yaml:"capabilityProfile"`
	SourcePlan        contract.ResolvedSourcePlan `json:"sourcePlan" yaml:"sourcePlan"`
}

type BackendService struct {
	HealthPort int `json:"healthPort" yaml:"healthPort"`
	APIPort    int `json:"apiPort" yaml:"apiPort"`
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

func (g *Generator) Generate(deployment otel.Deployment, capabilityProfile profile.ClusterProfile, sourcePlan contract.ResolvedSourcePlan) ([]byte, error) {
	values, err := g.Build(deployment, capabilityProfile, sourcePlan)
	if err != nil {
		return nil, err
	}

	payload, err := yaml.Marshal(values)
	if err != nil {
		return nil, fmt.Errorf("marshal helm values: %w", err)
	}
	return payload, nil
}

func (g *Generator) Build(deployment otel.Deployment, capabilityProfile profile.ClusterProfile, sourcePlan contract.ResolvedSourcePlan) (*Values, error) {
	agentConfig, err := yaml.Marshal(deployment.AgentConfig)
	if err != nil {
		return nil, fmt.Errorf("marshal agent config: %w", err)
	}

	gatewayConfig, err := yaml.Marshal(deployment.GatewayConfig)
	if err != nil {
		return nil, fmt.Errorf("marshal gateway config: %w", err)
	}

	logging := LoggingSpec{
		Format: DefaultLogFormat,
		Level:  DefaultLogLevel,
	}

	return &Values{
		Global: GlobalValues{
			ClusterName: deployment.ClusterName,
			Namespace:   deployment.Namespace,
			ReleaseName: g.options.ReleaseName,
			ChartPath:   g.options.ChartPath,
			Image:       g.options.CollectorImage,
			Logging:     logging,
		},
		EnabledSignals: deployment.EnabledSignals,
		Namespace: NamespaceSpec{
			Create: true,
			Name:   deployment.Namespace,
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
				FeatureGates:  []string{},
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
			Enabled:  deployment.Backend.Enabled,
			GRPCPort: DefaultBackendGRPCPort,
			Image:    g.options.BackendImage,
			Service: BackendService{
				HealthPort: DefaultBackendHealthPort,
				APIPort:    DefaultBackendAPIPort,
			},
			SourceContract: SourceContractValues{
				CapabilityProfile: capabilityProfile,
				SourcePlan:        sourcePlan,
			},
		},
		CollectorDebug: CollectorDebugValues{
			Enabled:        false,
			ExporterID:     deployment.DebugExporter.ID,
			ExporterConfig: deployment.DebugExporter.Config,
		},
		Storage: StorageValues{
			Backend:         "memory",
			PostgreSQL:      StoragePostgreSQLValues{},
			VictoriaMetrics: StorageVictoriaMetricsValues{},
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
