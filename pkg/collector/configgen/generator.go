package configgen

import (
	"fmt"

	"sigs.k8s.io/yaml"

	"github.com/erickivel/karthos/pkg/adapters/otel"
	"github.com/erickivel/karthos/pkg/discovery/profile"
	"github.com/erickivel/karthos/pkg/sources/contract"
)

type Generator struct {
	options GeneratorOptions
}

type GeneratorOptions struct {
	Runtime RuntimeSettings
}

// RuntimeSettings contains the explicit deployment inputs that may override
// chart defaults. Collector configuration and the source contract are always
// generated, never accepted through this type.
type RuntimeSettings struct {
	CollectorImage ImageValues
	BackendImage   ImageValues
	FrontendImage  ImageValues
	Storage        StorageSettings
}

type StorageBackend string

const (
	StorageBackendMemory   StorageBackend = "memory"
	StorageBackendExternal StorageBackend = "external"
)

type StoreMode string

const (
	StoreModeBundled StoreMode = "bundled"
	StoreModeRemote  StoreMode = "remote"
)

// StorageSettings selects the supported external dependency combinations.
// Empty settings intentionally leave the chart's memory default unchanged.
type StorageSettings struct {
	Backend         StorageBackend
	PostgreSQL      StoreSettings
	VictoriaMetrics StoreSettings
}

type StoreSettings struct {
	Mode     StoreMode
	Endpoint string
}

// Values is a Helm values overlay. It contains discovery-derived configuration
// and explicitly requested runtime overrides; static deployment policy stays
// in the chart's values.yaml.
type Values struct {
	Global         GlobalValues    `json:"global" yaml:"global"`
	EnabledSignals []string        `json:"enabledSignals,omitempty" yaml:"enabledSignals,omitempty"`
	Agent          AgentValues     `json:"agent" yaml:"agent"`
	Gateway        GatewayValues   `json:"gateway" yaml:"gateway"`
	Backend        BackendValues   `json:"backend" yaml:"backend"`
	Frontend       *FrontendValues `json:"frontend,omitempty" yaml:"frontend,omitempty"`
	Storage        *StorageValues  `json:"storage,omitempty" yaml:"storage,omitempty"`
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
	ClusterName string       `json:"clusterName" yaml:"clusterName"`
	Image       *ImageValues `json:"image,omitempty" yaml:"image,omitempty"`
}

type ImageValues struct {
	Repository string `json:"repository,omitempty" yaml:"repository,omitempty"`
	Tag        string `json:"tag,omitempty" yaml:"tag,omitempty"`
	PullPolicy string `json:"pullPolicy,omitempty" yaml:"pullPolicy,omitempty"`
}

type ComponentValues struct {
	CollectorYAML string `json:"collectorConfig" yaml:"collectorConfig"`
}

type AgentValues struct {
	ComponentValues `json:",inline" yaml:",inline"`
}

type GatewayValues struct {
	ComponentValues `json:",inline" yaml:",inline"`
}

type BackendValues struct {
	SourceContract SourceContractValues `json:"sourceContract" yaml:"sourceContract"`
	Image          *ImageValues         `json:"image,omitempty" yaml:"image,omitempty"`
}

type SourceContractValues struct {
	CapabilityProfile profile.ClusterProfile      `json:"capabilityProfile" yaml:"capabilityProfile"`
	SourcePlan        contract.ResolvedSourcePlan `json:"sourcePlan" yaml:"sourcePlan"`
}

type FrontendValues struct {
	Image ImageValues `json:"image" yaml:"image"`
}

func NewGenerator() *Generator { return &Generator{} }

func NewGeneratorWithOptions(options GeneratorOptions) *Generator {
	return &Generator{options: options}
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

	storage, err := g.options.Runtime.Storage.values()
	if err != nil {
		return nil, err
	}

	values := &Values{
		Global:         GlobalValues{ClusterName: deployment.ClusterName},
		EnabledSignals: deployment.EnabledSignals,
		Agent:          AgentValues{ComponentValues: ComponentValues{CollectorYAML: string(agentConfig)}},
		Gateway:        GatewayValues{ComponentValues: ComponentValues{CollectorYAML: string(gatewayConfig)}},
		Backend: BackendValues{SourceContract: SourceContractValues{
			CapabilityProfile: capabilityProfile,
			SourcePlan:        sourcePlan,
		}},
		Storage: storage,
	}
	if image := optionalImage(g.options.Runtime.CollectorImage); image != nil {
		values.Global.Image = image
	}
	if image := optionalImage(g.options.Runtime.BackendImage); image != nil {
		values.Backend.Image = image
	}
	if image := optionalImage(g.options.Runtime.FrontendImage); image != nil {
		values.Frontend = &FrontendValues{Image: *image}
	}
	return values, nil
}

func optionalImage(image ImageValues) *ImageValues {
	if image.Repository == "" && image.Tag == "" && image.PullPolicy == "" {
		return nil
	}
	return &image
}

func (s StorageSettings) values() (*StorageValues, error) {
	if s.Backend == "" {
		if s.PostgreSQL.Mode != "" || s.PostgreSQL.Endpoint != "" || s.VictoriaMetrics.Mode != "" || s.VictoriaMetrics.Endpoint != "" {
			return nil, fmt.Errorf("storage backend is required when store settings are supplied")
		}
		return nil, nil
	}
	if s.Backend == StorageBackendMemory {
		if s.PostgreSQL.Mode != "" || s.PostgreSQL.Endpoint != "" || s.VictoriaMetrics.Mode != "" || s.VictoriaMetrics.Endpoint != "" {
			return nil, fmt.Errorf("store settings are only supported for external storage")
		}
		return &StorageValues{Backend: string(StorageBackendMemory)}, nil
	}
	if s.Backend != StorageBackendExternal {
		return nil, fmt.Errorf("storage backend must be memory or external")
	}
	postgres, err := postgresValues(s.PostgreSQL)
	if err != nil {
		return nil, err
	}
	victoria, err := victoriaMetricsValues(s.VictoriaMetrics)
	if err != nil {
		return nil, err
	}
	return &StorageValues{Backend: string(StorageBackendExternal), PostgreSQL: postgres, VictoriaMetrics: victoria}, nil
}

func postgresValues(settings StoreSettings) (StoragePostgreSQLValues, error) {
	switch settings.Mode {
	case StoreModeBundled:
		if settings.Endpoint != "" {
			return StoragePostgreSQLValues{}, fmt.Errorf("postgresql bundled mode does not accept a DSN")
		}
		return StoragePostgreSQLValues{Enabled: true}, nil
	case StoreModeRemote:
		if settings.Endpoint == "" {
			return StoragePostgreSQLValues{}, fmt.Errorf("postgresql remote mode requires a DSN")
		}
		return StoragePostgreSQLValues{DSN: settings.Endpoint}, nil
	case "":
		return StoragePostgreSQLValues{}, fmt.Errorf("postgresql mode is required for external storage")
	default:
		return StoragePostgreSQLValues{}, fmt.Errorf("postgresql mode must be bundled or remote")
	}
}

func victoriaMetricsValues(settings StoreSettings) (StorageVictoriaMetricsValues, error) {
	switch settings.Mode {
	case StoreModeBundled:
		if settings.Endpoint != "" {
			return StorageVictoriaMetricsValues{}, fmt.Errorf("victoriametrics bundled mode does not accept a URL")
		}
		return StorageVictoriaMetricsValues{Enabled: true}, nil
	case StoreModeRemote:
		if settings.Endpoint == "" {
			return StorageVictoriaMetricsValues{}, fmt.Errorf("victoriametrics remote mode requires a URL")
		}
		return StorageVictoriaMetricsValues{URL: settings.Endpoint}, nil
	case "":
		return StorageVictoriaMetricsValues{}, fmt.Errorf("victoriametrics mode is required for external storage")
	default:
		return StorageVictoriaMetricsValues{}, fmt.Errorf("victoriametrics mode must be bundled or remote")
	}
}
