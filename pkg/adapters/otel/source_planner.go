package otel

import (
	"context"
	"fmt"

	"github.com/erickivel/oksg/pkg/discovery/profile"
)

const (
	DefaultNamespace           = "oksg-system"
	DefaultGatewayGRPCEndpoint = "otel-collector-gateway:4317"
	DefaultGatewayHTTPEndpoint = "0.0.0.0:4318"
	DefaultBackendGRPCEndpoint = "oksg-backend:4317"
	DefaultBackendHTTPEndpoint = "0.0.0.0:4318"
	DefaultHealthEndpoint      = "0.0.0.0:13133"
	DefaultClusterName         = "default-cluster"
)

type SourcePlanner interface {
	Plan(ctx context.Context, profile *profile.ClusterProfile) (*Plan, error)
}

type Plan struct {
	ClusterName    string
	Namespace      string
	EnabledSignals []string
	Backend        BackendConfig
	AgentConfig    CollectorConfig
	GatewayConfig  CollectorConfig
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

	enabledSignals := []string{"metric"}
	agentReceivers := map[string]any{
		"kubeletstats": map[string]any{
			"collection_interval":  "30s",
			"auth_type":            "serviceAccount",
			"endpoint":             "https://${env:K8S_NODE_NAME}:10250",
			"insecure_skip_verify": true,
			"metric_groups":        []string{"node", "pod", "container", "volume"},
		},
		"hostmetrics": map[string]any{
			"collection_interval": "30s",
			"root_path":           "/hostfs",
			"scrapers": map[string]any{
				"cpu":        map[string]any{},
				"memory":     map[string]any{},
				"disk":       map[string]any{},
				"filesystem": map[string]any{},
				"network":    map[string]any{},
				"load":       map[string]any{},
				"paging":     map[string]any{},
			},
		},
	}

	pipelineReceivers := []string{"kubeletstats", "hostmetrics"}
	prometheusTargets := prometheusTargets(clusterProfile.Sources)
	if len(prometheusTargets) > 0 {
		agentReceivers["prometheus"] = map[string]any{
			"config": map[string]any{
				"scrape_configs": buildPrometheusScrapeConfigs(prometheusTargets),
			},
		}
		pipelineReceivers = append(pipelineReceivers, "prometheus")
	}
	otlpSignals := otlpSignals(clusterProfile.Sources)
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
	if otlpSignals.metrics {
		pipelineReceivers = append(pipelineReceivers, "otlp")
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
			"k8sattributes": map[string]any{
				"auth_type":   "serviceAccount",
				"passthrough": false,
			},
			"batch": map[string]any{},
		},
		Exporters: map[string]any{
			"otlp": map[string]any{
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
					Processors: []string{"resource", "k8sattributes", "batch"},
					Exporters:  []string{"otlp"},
				},
			},
		},
	}
	if otlpSignals.traces {
		agentConfig.Service.Pipelines["traces"] = PipelineConfig{
			Receivers:  []string{"otlp"},
			Processors: []string{"resource", "k8sattributes", "batch"},
			Exporters:  []string{"otlp"},
		}
	}

	// Gateway receivers: always include otlp and k8s_cluster.
	gatewayReceivers := map[string]any{
		"otlp": map[string]any{
			"protocols": map[string]any{
				"grpc": map[string]any{
					"endpoint": "0.0.0.0:4317",
				},
				"http": map[string]any{
					"endpoint": DefaultGatewayHTTPEndpoint,
				},
			},
		},
		"k8s_cluster": map[string]any{
			"collection_interval":         "30s",
			"node_conditions_to_report":   []string{"Ready", "MemoryPressure", "DiskPressure"},
			"allocatable_types_to_report": []string{"cpu", "memory", "ephemeral-storage"},
			"metrics": map[string]any{
				"k8s.node.condition": map[string]any{
					"enabled": true,
				},
			},
		},
	}
	gatewayMetricsReceivers := []string{"otlp", "k8s_cluster"}
	var gatewayConnectors map[string]any
	if otlpSignals.traces {
		gatewayConnectors = map[string]any{
			"spanmetrics": map[string]any{
				"histogram": map[string]any{
					"explicit": map[string]any{
						"buckets": []string{"100ms", "250ms", "500ms", "1s", "2s", "5s"},
					},
				},
				"dimensions": []map[string]any{
					{"name": "http.method", "default": "GET"},
					{"name": "http.status_code"},
				},
				"exemplars": map[string]any{"enabled": true},
			},
			"servicegraph": map[string]any{
				"latency_histogram_buckets": []string{"100ms", "250ms", "500ms", "1s", "5s"},
				"dimensions":                []string{"http.method"},
				"store": map[string]any{
					"ttl":       "2s",
					"max_items": 1000,
				},
			},
		}
		gatewayMetricsReceivers = append(gatewayMetricsReceivers, "spanmetrics", "servicegraph")
	}

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
			"batch": map[string]any{},
		},
		Connectors: gatewayConnectors,
		Exporters: map[string]any{
			"otlp": map[string]any{
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
					Processors: []string{"memory_limiter", "batch"},
					Exporters:  []string{"otlp"},
				},
			},
		},
	}
	if otlpSignals.traces {
		gatewayConfig.Service.Pipelines["traces"] = PipelineConfig{
			Receivers:  []string{"otlp"},
			Processors: []string{"memory_limiter", "batch"},
			Exporters:  []string{"otlp", "spanmetrics", "servicegraph"},
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
		ClusterName:    clusterLabel,
		Namespace:      p.namespace,
		EnabledSignals: enabledSignals,
		Backend: BackendConfig{
			Enabled:      true,
			GRPCEndpoint: DefaultBackendGRPCEndpoint,
			HTTPEndpoint: DefaultBackendHTTPEndpoint,
		},
		AgentConfig:   agentConfig,
		GatewayConfig: gatewayConfig,
	}, nil
}

type sourceSignals struct {
	metrics bool
	traces  bool
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

func otlpSignals(sources []profile.ObservabilitySource) sourceSignals {
	var signals sourceSignals
	for _, source := range sources {
		if source.Type != "otlp" || !source.Present || source.OTLP == nil {
			continue
		}
		signals.metrics = signals.metrics || source.OTLP.MetricsEnabled
		signals.traces = signals.traces || source.OTLP.TracesEnabled
	}
	return signals
}
