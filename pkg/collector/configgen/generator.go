package configgen

import (
	"fmt"

	"sigs.k8s.io/yaml"

	"github.com/erickivel/oksg/pkg/adapters/otel"
)

const (
	DefaultReleaseName = "oksg-otel"
	DefaultChartPath   = "./deploy/helm/oksg-otel"
	DefaultLogLevel    = "info"
	DefaultLogFormat   = "json"
)

type Generator struct{}

type Values struct {
	Global    GlobalValues  `json:"global" yaml:"global"`
	Agent     AgentValues   `json:"agent" yaml:"agent"`
	Gateway   GatewayValues `json:"gateway" yaml:"gateway"`
	Backend   BackendValues `json:"backend" yaml:"backend"`
	Secrets   SecretsValues `json:"secrets" yaml:"secrets"`
	Rollout   RolloutValues `json:"rollout" yaml:"rollout"`
	Namespace NamespaceSpec `json:"namespace" yaml:"namespace"`
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
	RuntimeEnv    map[string]string `json:"runtimeEnv,omitempty" yaml:"runtimeEnv,omitempty"`
	CollectorYAML string            `json:"collectorConfig" yaml:"collectorConfig"`
}

type AgentValues struct {
	ComponentValues `json:",inline" yaml:",inline"`
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
	Type string `json:"type" yaml:"type"`
	Port int    `json:"port" yaml:"port"`
}

type BackendValues struct {
	Enabled bool `json:"enabled" yaml:"enabled"`
}

type SecretsValues struct {
	ExistingSecretName string `json:"existingSecretName,omitempty" yaml:"existingSecretName,omitempty"`
}

type RolloutValues struct {
	UseConfigChecksum bool `json:"useConfigChecksum" yaml:"useConfigChecksum"`
}

func NewGenerator() *Generator {
	return &Generator{}
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
			ReleaseName: DefaultReleaseName,
			ChartPath:   DefaultChartPath,
			Image: ImageValues{
				Repository: "otel/opentelemetry-collector-contrib",
				Tag:        "0.111.0",
				PullPolicy: "IfNotPresent",
			},
			Logging: logging,
		},
		Namespace: NamespaceSpec{
			Create: true,
			Name:   plan.Namespace,
		},
		Agent: AgentValues{
			ComponentValues: ComponentValues{
				Enabled:       true,
				Logging:       logging,
				CollectorYAML: string(agentConfig),
			},
		},
		Gateway: GatewayValues{
			ComponentValues: ComponentValues{
				Enabled:       true,
				Logging:       logging,
				CollectorYAML: string(gatewayConfig),
			},
			Replicas: 1,
			Service: ServiceValues{
				Type: "ClusterIP",
				Port: 4317,
			},
		},
		Backend: BackendValues{
			Enabled: false,
		},
		Secrets: SecretsValues{},
		Rollout: RolloutValues{
			UseConfigChecksum: true,
		},
	}, nil
}
