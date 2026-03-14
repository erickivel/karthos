package otel

import (
	"context"
	"fmt"

	"github.com/erickivel/oksg/pkg/discovery/profile"
)

const (
	DefaultNamespace       = "oksg-system"
	DefaultGatewayEndpoint = "otel-collector-gateway:4317"
	DefaultHealthEndpoint  = "0.0.0.0:13133"
	DefaultClusterName     = "default-cluster"
)

type SourcePlanner interface {
	Plan(ctx context.Context, profile *profile.ClusterProfile) (*Plan, error)
}

type Plan struct {
	ClusterName   string
	Namespace     string
	AgentConfig   CollectorConfig
	GatewayConfig CollectorConfig
}

type CollectorConfig struct {
	Extensions map[string]any `json:"extensions,omitempty" yaml:"extensions,omitempty"`
	Receivers  map[string]any `json:"receivers" yaml:"receivers"`
	Processors map[string]any `json:"processors" yaml:"processors"`
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
}

func NewSourcePlanner(namespace string) SourcePlanner {
	if namespace == "" {
		namespace = DefaultNamespace
	}

	return &planner{namespace: namespace}
}

func (p *planner) Plan(_ context.Context, clusterProfile *profile.ClusterProfile) (*Plan, error) {
	if clusterProfile == nil {
		return nil, fmt.Errorf("cluster profile is required")
	}

	agentReceivers := map[string]any{
		"kubeletstats": map[string]any{
			"collection_interval":  "30s",
			"auth_type":            "serviceAccount",
			"endpoint":             "https://${env:K8S_NODE_NAME}:10250",
			"insecure_skip_verify": true,
			"metric_groups":        []string{"node", "pod", "container"},
		},
	}

	pipelineReceivers := []string{"kubeletstats"}
	prometheusTargets := prometheusTargets(clusterProfile.Sources)
	if len(prometheusTargets) > 0 {
		agentReceivers["prometheus"] = map[string]any{
			"config": map[string]any{
				"scrape_configs": buildPrometheusScrapeConfigs(prometheusTargets),
			},
		}
		pipelineReceivers = append(pipelineReceivers, "prometheus")
	}

	agentConfig := CollectorConfig{
		Extensions: map[string]any{
			"health_check": map[string]any{
				"endpoint": DefaultHealthEndpoint,
			},
		},
		Receivers: agentReceivers,
		Processors: map[string]any{
			"k8sattributes": map[string]any{
				"auth_type":   "serviceAccount",
				"passthrough": false,
			},
			"batch": map[string]any{},
		},
		Exporters: map[string]any{
			"otlp": map[string]any{
				"endpoint": DefaultGatewayEndpoint,
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
					Processors: []string{"k8sattributes", "batch"},
					Exporters:  []string{"otlp"},
				},
			},
		},
	}

	gatewayConfig := CollectorConfig{
		Extensions: map[string]any{
			"health_check": map[string]any{
				"endpoint": DefaultHealthEndpoint,
			},
		},
		Receivers: map[string]any{
			"otlp": map[string]any{
				"protocols": map[string]any{
					"grpc": map[string]any{
						"endpoint": "0.0.0.0:4317",
					},
				},
			},
		},
		Processors: map[string]any{
			"batch": map[string]any{},
		},
		Exporters: map[string]any{
			"debug": map[string]any{},
		},
		Service: ServiceConfig{
			Extensions: []string{"health_check"},
			Pipelines: map[string]PipelineConfig{
				"metrics": {
					Receivers:  []string{"otlp"},
					Processors: []string{"batch"},
					Exporters:  []string{"debug"},
				},
			},
		},
	}

	clusterLabel := clusterProfile.ClusterName
	if clusterLabel == "" {
		clusterLabel = string(clusterProfile.ClusterID)
	}
	if clusterLabel == "" {
		clusterLabel = DefaultClusterName
	}

	return &Plan{
		ClusterName:   clusterLabel,
		Namespace:     p.namespace,
		AgentConfig:   agentConfig,
		GatewayConfig: gatewayConfig,
	}, nil
}

func prometheusTargets(sources []profile.ObservabilitySource) []profile.PrometheusTarget {
	var targets []profile.PrometheusTarget
	for _, source := range sources {
		if source.Type != "prometheus" || !source.Present {
			continue
		}
		targets = append(targets, source.PrometheusTargets...)
	}
	return targets
}

func buildPrometheusScrapeConfigs(targets []profile.PrometheusTarget) []map[string]any {
	scrapeConfigs := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		scrapeConfigs = append(scrapeConfigs, map[string]any{
			"job_name":     fmt.Sprintf("%s-%s", target.Namespace, target.ServiceName),
			"metrics_path": target.Path,
			"static_configs": []map[string]any{
				{
					"targets": []string{
						fmt.Sprintf("%s.%s.svc.cluster.local:%d", target.ServiceName, target.Namespace, target.Port),
					},
					"labels": map[string]string{
						"service":   target.ServiceName,
						"namespace": target.Namespace,
					},
				},
			},
		})
	}
	return scrapeConfigs
}
