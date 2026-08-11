package otel

import (
	"context"
	"fmt"
	"sort"

	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/discovery/profile"
	"github.com/erickivel/oksg/pkg/sources"
)

const (
	DefaultNamespace           = "oksg-system"
	DefaultGatewayGRPCEndpoint = "otel-collector-gateway:4317"
	DefaultGatewayHTTPEndpoint = "0.0.0.0:4318"
	DefaultBackendGRPCEndpoint = "oksg-backend:4317"
	DefaultBackendHTTPEndpoint = "0.0.0.0:4318"
	DefaultHealthEndpoint      = "0.0.0.0:13133"
	DefaultClusterName         = "default-cluster"
	otlpGRPCExporter           = "otlp_grpc"
)

type SourcePlanner interface {
	Plan(ctx context.Context, profile *profile.ClusterProfile) (*Plan, error)
}

type Plan struct {
	ClusterName       string
	Namespace         string
	EnabledSignals    []string
	Backend           BackendConfig
	AgentConfig       CollectorConfig
	GatewayConfig     CollectorConfig
	CapabilityProfile profile.ClusterProfile
	SourcePlan        ResolvedSourcePlan
}

// ResolvedSourcePlan is the deployment contract shared by collector
// compilation and backend ingestion. It records the complete, deterministic
// result of source arbitration rather than asking runtime code to resolve it
// again from discovery evidence.
type ResolvedSourcePlan struct {
	ProfileFingerprint string             `json:"profile_fingerprint"`
	SelectedSources    []string           `json:"selected_sources"`
	MetricAssignments  metrics.Assignment `json:"metric_assignments"`
}

// Validate verifies that a mounted plan is a complete projection of the
// compiled catalog and module inventory. It deliberately does not rerun source
// arbitration: the profile fingerprint binds this checked artifact to the
// discovery snapshot that produced it.
func (p ResolvedSourcePlan) Validate(capabilityProfile *profile.ClusterProfile, modules sources.Registry) error {
	if err := capabilityProfile.Validate(modules.IDs()); err != nil {
		return fmt.Errorf("validate capability profile: %w", err)
	}

	metricSources := make(map[string]struct{}, len(modules.MetricIDs()))
	for _, sourceID := range modules.MetricIDs() {
		metricSources[sourceID] = struct{}{}
	}
	selected := make(map[string]struct{}, len(p.SelectedSources))
	for _, sourceID := range p.SelectedSources {
		if _, ok := metricSources[sourceID]; !ok {
			return fmt.Errorf("source plan selects unknown telemetry source %q", sourceID)
		}
		if _, duplicate := selected[sourceID]; duplicate {
			return fmt.Errorf("source plan selects source %q more than once", sourceID)
		}
		if !capabilityProfile.IsAvailable(sourceID) {
			return fmt.Errorf("source plan selects unavailable source %q", sourceID)
		}
		selected[sourceID] = struct{}{}
	}

	catalog := metrics.CatalogByName()
	selectedUsed := make(map[string]bool, len(selected))
	for metricName, sourceID := range p.MetricAssignments {
		metric, ok := catalog[metricName]
		if !ok {
			return fmt.Errorf("source plan assigns unknown catalog metric %q", metricName)
		}
		if _, ok := selected[sourceID]; !ok {
			return fmt.Errorf("source plan assigns metric %q to unselected source %q", metricName, sourceID)
		}
		if !metricHasSource(metric, sourceID) {
			return fmt.Errorf("source plan assigns metric %q to unsupported source %q", metricName, sourceID)
		}
		selectedUsed[sourceID] = true
	}
	for sourceID := range selected {
		if !selectedUsed[sourceID] {
			return fmt.Errorf("source plan selects unused source %q", sourceID)
		}
	}

	for metricName, metric := range catalog {
		available := false
		for _, source := range metric.Sources {
			if capabilityProfile.IsAvailable(source.ID) {
				available = true
				break
			}
		}
		_, assigned := p.MetricAssignments[metricName]
		if available && !assigned {
			return fmt.Errorf("source plan must assign every available catalog metric: %q", metricName)
		}
		if !available && assigned {
			return fmt.Errorf("source plan assigns unavailable catalog metric %q", metricName)
		}
	}

	return nil
}

func metricHasSource(metric metrics.CanonicalMetric, sourceID string) bool {
	for _, source := range metric.Sources {
		if source.ID == sourceID {
			return true
		}
	}
	return false
}

type BackendConfig struct {
	Enabled      bool
	GRPCEndpoint string
	HTTPEndpoint string
}

type CollectorConfig struct {
	Extensions map[string]any `json:"extensions,omitempty" yaml:"extensions,omitempty"`
	Receivers  map[string]any `json:"receivers" yaml:"receivers"`
	Processors map[string]any `json:"processors" yaml:"processors"`
	Connectors map[string]any `json:"connectors,omitempty" yaml:"connectors,omitempty"`
	Exporters  map[string]any `json:"exporters" yaml:"exporters"`
	Service    ServiceConfig  `json:"service" yaml:"service"`
}

type ServiceConfig struct {
	Extensions []string                  `json:"extensions,omitempty" yaml:"extensions,omitempty"`
	Pipelines  map[string]PipelineConfig `json:"pipelines" yaml:"pipelines"`
}

type PipelineConfig struct {
	Receivers  []string `json:"receivers" yaml:"receivers"`
	Processors []string `json:"processors" yaml:"processors"`
	Exporters  []string `json:"exporters" yaml:"exporters"`
}

type planner struct {
	namespace string
	modules   sources.Registry
}

func NewSourcePlanner(namespace string) SourcePlanner {
	if namespace == "" {
		namespace = DefaultNamespace
	}

	return &planner{namespace: namespace, modules: sources.DefaultRegistry()}
}

func (p *planner) Plan(_ context.Context, clusterProfile *profile.ClusterProfile) (*Plan, error) {
	if clusterProfile == nil {
		return nil, fmt.Errorf("cluster profile is required")
	}
	if err := clusterProfile.Validate(sources.DefaultRegistry().IDs()); err != nil {
		return nil, fmt.Errorf("validate capability profile: %w", err)
	}

	assignment, selected, err := metrics.Resolve(metrics.DefaultCatalog(), p.modules.MetricIDs(), clusterProfile.IsAvailable)
	if err != nil {
		return nil, fmt.Errorf("resolve sources: %w", err)
	}
	contributions, err := p.modules.Contribute(selected, clusterProfile)
	if err != nil {
		return nil, fmt.Errorf("compile source contributions: %w", err)
	}
	profileFingerprint, err := clusterProfile.Fingerprint()
	if err != nil {
		return nil, fmt.Errorf("fingerprint capability profile: %w", err)
	}
	selectedSourceIDs := make([]string, 0, len(selected))
	for sourceID := range selected {
		selectedSourceIDs = append(selectedSourceIDs, sourceID)
	}
	sort.Strings(selectedSourceIDs)

	enabledSignals := []string{"metric"}
	agentReceivers := contributions.AgentReceivers
	pipelineReceivers := contributions.AgentMetricReceivers
	otlpSignals := otlpSignals(clusterProfile)
	if otlpSignals.metrics || otlpSignals.traces {
		agentReceivers["otlp"] = map[string]any{
			"protocols": map[string]any{
				"grpc": map[string]any{
					"endpoint": "0.0.0.0:4317",
				},
				"http": map[string]any{
					"endpoint": DefaultGatewayHTTPEndpoint,
				},
			},
		}
	}
	if otlpSignals.traces {
		enabledSignals = append(enabledSignals, "trace")
	}

	agentConfig := CollectorConfig{
		Extensions: map[string]any{
			"health_check": map[string]any{
				"endpoint": DefaultHealthEndpoint,
			},
		},
		Receivers: agentReceivers,
		Processors: map[string]any{
			"resource": map[string]any{
				"attributes": []map[string]any{
					{
						"key":    "k8s.node.name",
						"value":  "${env:K8S_NODE_NAME}",
						"action": "insert",
					},
				},
			},
			"k8s_attributes": map[string]any{
				"auth_type":   "serviceAccount",
				"passthrough": false,
			},
			oksgSourceTaggerProcessor: sourceTagger(contributions.AgentSourceStamps),
			"batch":                   map[string]any{},
		},
		Exporters: map[string]any{
			otlpGRPCExporter: map[string]any{
				"endpoint": DefaultGatewayGRPCEndpoint,
				"tls": map[string]any{
					"insecure": true,
				},
			},
		},
		Service: ServiceConfig{
			Extensions: []string{"health_check"},
			Pipelines: map[string]PipelineConfig{
				"metrics": {
					Receivers:  pipelineReceivers,
					Processors: []string{"resource", "k8s_attributes", oksgSourceTaggerProcessor, "batch"},
					Exporters:  []string{otlpGRPCExporter},
				},
			},
		},
	}
	if otlpSignals.metrics {
		agentConfig.Service.Pipelines["metrics/otlp"] = PipelineConfig{
			Receivers:  []string{"otlp"},
			Processors: []string{"resource", "k8s_attributes", "batch"},
			Exporters:  []string{otlpGRPCExporter},
		}
	}
	if otlpSignals.traces {
		agentConfig.Service.Pipelines["traces"] = PipelineConfig{
			Receivers:  []string{"otlp"},
			Processors: []string{"resource", "k8s_attributes", "batch"},
			Exporters:  []string{otlpGRPCExporter},
		}
	}

	// Gateway transport is generic. Source modules contribute metric receivers.
	gatewayReceivers := contributions.GatewayReceivers
	gatewayReceivers["otlp"] = map[string]any{
		"protocols": map[string]any{
			"grpc": map[string]any{
				"endpoint": "0.0.0.0:4317",
			},
			"http": map[string]any{
				"endpoint": DefaultGatewayHTTPEndpoint,
			},
		},
	}
	gatewayMetricsReceivers := contributions.GatewayMetricReceivers
	gatewayConnectors := contributions.GatewayConnectors
	deployServicegraph := selected.Has(sources.IDTracesServicegraph)

	gatewayConfig := CollectorConfig{
		Extensions: map[string]any{
			"health_check": map[string]any{
				"endpoint": DefaultHealthEndpoint,
			},
		},
		Receivers: gatewayReceivers,
		Processors: map[string]any{
			"memory_limiter": map[string]any{
				"check_interval":         "1s",
				"limit_percentage":       75,
				"spike_limit_percentage": 15,
			},
			oksgSourceTaggerProcessor: sourceTagger(contributions.GatewaySourceStamps),
			"batch":                   map[string]any{},
		},
		Connectors: gatewayConnectors,
		Exporters: map[string]any{
			otlpGRPCExporter: map[string]any{
				"endpoint": DefaultBackendGRPCEndpoint,
				"tls": map[string]any{
					"insecure": true,
				},
				"sending_queue": map[string]any{
					"enabled": true,
				},
				"retry_on_failure": map[string]any{
					"enabled": true,
				},
			},
		},
		Service: ServiceConfig{
			Extensions: []string{"health_check"},
			Pipelines: map[string]PipelineConfig{
				"metrics": {
					Receivers:  gatewayMetricsReceivers,
					Processors: []string{"memory_limiter", oksgSourceTaggerProcessor, "batch"},
					Exporters:  []string{otlpGRPCExporter},
				},
				// All forwarded agent metrics and direct external OTLP metrics enter
				// through this receiver. Agent-owned datapoints are already stamped at
				// origin; direct producers remain untagged and are rejected by the
				// backend instead of being mislabeled by metric name.
				"metrics/otlp": {
					Receivers:  []string{"otlp"},
					Processors: []string{"memory_limiter", "batch"},
					Exporters:  []string{otlpGRPCExporter},
				},
			},
		},
	}
	if otlpSignals.traces {
		traceExporters := []string{otlpGRPCExporter}
		if deployServicegraph {
			traceExporters = append(traceExporters, "service_graph")
		}
		gatewayConfig.Service.Pipelines["traces"] = PipelineConfig{
			Receivers:  []string{"otlp"},
			Processors: []string{"memory_limiter", "batch"},
			Exporters:  traceExporters,
		}
	}

	clusterLabel := clusterProfile.ClusterName
	if clusterLabel == "" {
		clusterLabel = string(clusterProfile.ClusterID)
	}
	if clusterLabel == "" {
		clusterLabel = DefaultClusterName
	}

	return &Plan{
		ClusterName:       clusterLabel,
		Namespace:         p.namespace,
		EnabledSignals:    enabledSignals,
		CapabilityProfile: *clusterProfile,
		Backend: BackendConfig{
			Enabled:      true,
			GRPCEndpoint: DefaultBackendGRPCEndpoint,
			HTTPEndpoint: DefaultBackendHTTPEndpoint,
		},
		AgentConfig:   agentConfig,
		GatewayConfig: gatewayConfig,
		SourcePlan: ResolvedSourcePlan{
			ProfileFingerprint: profileFingerprint,
			SelectedSources:    selectedSourceIDs,
			MetricAssignments:  assignment,
		},
	}, nil
}

type sourceSignals struct {
	metrics bool
	traces  bool
}

// oksgSourceTaggerProcessor names the OTel transform processor that stamps
// `oksg_source` onto each datapoint based on the receiver-determined metric
// name space. The istio prometheus scrape already sets `oksg_source` via
// metric_relabel_configs; the where clauses below skip overwriting that.
const oksgSourceTaggerProcessor = "transform/oksg-source"

// sourceTagger wraps source-module-owned OTTL stamp statements in the generic
// transform processor shape. The compiler never derives a source from a metric
// name itself.
func sourceTagger(statements []string) map[string]any {
	return map[string]any{
		"metric_statements": []map[string]any{
			{
				"context":    "datapoint",
				"statements": statements,
			},
		},
	}
}

func otlpSignals(clusterProfile *profile.ClusterProfile) sourceSignals {
	metricsEnabled, tracesEnabled := clusterProfile.OTLPSignals()
	return sourceSignals{metrics: metricsEnabled, traces: tracesEnabled}
}
