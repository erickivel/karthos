// Package otel translates provider-neutral collection requirements into
// OpenTelemetry Collector configuration.
package otel

import (
	"fmt"
	"sort"
	"strings"

	collectorspec "github.com/erickivel/oksg/pkg/collector/spec"
)

const (
	DefaultGatewayGRPCEndpoint = "otel-collector-gateway:4317"
	DefaultGatewayHTTPEndpoint = "0.0.0.0:4318"
	DefaultBackendGRPCEndpoint = "oksg-backend:4317"
	DefaultBackendHTTPEndpoint = "0.0.0.0:4318"
	DefaultHealthEndpoint      = "0.0.0.0:13133"
	otlpGRPCExporter           = "otlp_grpc"
	debugExporterID            = "debug/oksg"
	oksgSourceTaggerProcessor  = "transform/oksg-source"
)

type BackendConfig struct {
	Enabled      bool
	GRPCEndpoint string
	HTTPEndpoint string
}

// DebugExporter is the OpenTelemetry-specific development diagnostic output.
// Helm controls whether it is attached to generated pipelines; this adapter
// owns its Collector component name and configuration shape.
type DebugExporter struct {
	ID     string
	Config map[string]any
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

// Deployment is the OpenTelemetry-specific result consumed by Helm values
// generation. The source contract stays outside this implementation adapter.
type Deployment struct {
	ClusterName    string
	Namespace      string
	EnabledSignals []string
	Backend        BackendConfig
	DebugExporter  DebugExporter
	AgentConfig    CollectorConfig
	GatewayConfig  CollectorConfig
}

type Compiler struct{}

func NewCompiler() *Compiler { return &Compiler{} }

func (c *Compiler) Compile(plan collectorspec.CollectionPlan) (Deployment, error) {
	if plan.Namespace == "" {
		return Deployment{}, fmt.Errorf("collection plan namespace is required")
	}
	if plan.ClusterName == "" {
		return Deployment{}, fmt.Errorf("collection plan cluster name is required")
	}
	agentConfig := compileAgent(plan.Agent)
	gatewayConfig := compileGateway(plan.Gateway)
	return Deployment{
		ClusterName: plan.ClusterName, Namespace: plan.Namespace,
		EnabledSignals: append([]string(nil), plan.EnabledSignals...),
		Backend:        BackendConfig{Enabled: true, GRPCEndpoint: DefaultBackendGRPCEndpoint, HTTPEndpoint: DefaultBackendHTTPEndpoint},
		DebugExporter:  debugExporter(),
		AgentConfig:    agentConfig, GatewayConfig: gatewayConfig,
	}, nil
}

func debugExporter() DebugExporter {
	return DebugExporter{
		ID: debugExporterID,
		Config: map[string]any{
			"verbosity":           "detailed",
			"use_internal_logger": false,
			"output_paths":        []string{"stdout"},
		},
	}
}

func compileAgent(requirements collectorspec.AgentRequirements) CollectorConfig {
	receivers := map[string]any{}
	metricReceivers := make([]string, 0, 3)
	if requirements.Kubelet != nil {
		receivers["kubelet_stats"] = map[string]any{"collection_interval": requirements.Kubelet.CollectionInterval, "auth_type": "serviceAccount", "endpoint": "https://${env:K8S_NODE_NAME}:10250", "insecure_skip_verify": true, "metric_groups": requirements.Kubelet.MetricGroups, "extra_metadata_labels": requirements.Kubelet.ExtraMetadataLabels}
		metricReceivers = append(metricReceivers, "kubelet_stats")
	}
	if requirements.HostOS != nil {
		scrapers := map[string]any{}
		for _, scraper := range requirements.HostOS.Scrapers {
			switch scraper {
			case "cpu":
				scrapers[scraper] = map[string]any{"metrics": map[string]any{"system.cpu.utilization": map[string]any{"enabled": true}}}
			case "filesystem":
				scrapers[scraper] = map[string]any{"metrics": map[string]any{"system.filesystem.utilization": map[string]any{"enabled": true}}}
			default:
				scrapers[scraper] = map[string]any{}
			}
		}
		receivers["host_metrics"] = map[string]any{"collection_interval": requirements.HostOS.CollectionInterval, "root_path": requirements.HostOS.RootPath, "scrapers": scrapers}
		metricReceivers = append(metricReceivers, "host_metrics")
	}
	if requirements.Istio != nil {
		receivers["prometheus"] = istioReceiver(requirements.Istio)
		metricReceivers = append(metricReceivers, "prometheus")
	}
	if requirements.OTLP.Metrics || requirements.OTLP.Traces {
		receivers["otlp"] = otlpReceiver()
	}
	sort.Strings(metricReceivers)
	config := CollectorConfig{
		Extensions: map[string]any{"health_check": map[string]any{"endpoint": DefaultHealthEndpoint}},
		Receivers:  receivers,
		Processors: map[string]any{
			"resource":                map[string]any{"attributes": []map[string]any{{"key": "k8s.node.name", "value": "${env:K8S_NODE_NAME}", "action": "insert"}}},
			"k8s_attributes":          map[string]any{"auth_type": "serviceAccount", "passthrough": false},
			oksgSourceTaggerProcessor: sourceTagger(requirements.SourceStamps), "batch": map[string]any{},
		},
		Exporters: map[string]any{otlpGRPCExporter: agentExporter()},
		Service: ServiceConfig{Extensions: []string{"health_check"}, Pipelines: map[string]PipelineConfig{
			"metrics": {Receivers: metricReceivers, Processors: []string{"resource", "k8s_attributes", oksgSourceTaggerProcessor, "batch"}, Exporters: []string{otlpGRPCExporter}},
		}},
	}
	if requirements.OTLP.Metrics {
		config.Service.Pipelines["metrics/otlp"] = PipelineConfig{Receivers: []string{"otlp"}, Processors: []string{"resource", "k8s_attributes", "batch"}, Exporters: []string{otlpGRPCExporter}}
	}
	if requirements.OTLP.Traces {
		config.Service.Pipelines["traces"] = PipelineConfig{Receivers: []string{"otlp"}, Processors: []string{"resource", "k8s_attributes", "batch"}, Exporters: []string{otlpGRPCExporter}}
	}
	return config
}

func compileGateway(requirements collectorspec.GatewayRequirements) CollectorConfig {
	receivers := map[string]any{}
	metricReceivers := make([]string, 0, 3)
	if requirements.KubeCluster != nil {
		metricsEnabled := map[string]any{}
		for _, name := range requirements.KubeCluster.EnabledMetricNames {
			metricsEnabled[name] = map[string]any{"enabled": true}
		}
		receivers["k8s_cluster"] = map[string]any{"collection_interval": requirements.KubeCluster.CollectionInterval, "node_conditions_to_report": requirements.KubeCluster.NodeConditions, "metrics": metricsEnabled}
		metricReceivers = append(metricReceivers, "k8s_cluster")
	}
	if requirements.Prometheus != nil {
		receivers["prometheus"] = prometheusReceiver(requirements.Prometheus)
		metricReceivers = append(metricReceivers, "prometheus")
	}
	connectors := map[string]any{}
	if requirements.ServiceGraph != nil {
		connectors["service_graph"] = map[string]any{"metrics_flush_interval": requirements.ServiceGraph.MetricsFlushInterval, "latency_histogram_buckets": requirements.ServiceGraph.LatencyHistogramBuckets, "dimensions": requirements.ServiceGraph.Dimensions, "store": map[string]any{"ttl": requirements.ServiceGraph.StoreTTL, "max_items": requirements.ServiceGraph.StoreMaxItems}}
		metricReceivers = append(metricReceivers, "service_graph")
	}
	if requirements.OTLP.Metrics || requirements.OTLP.Traces {
		receivers["otlp"] = otlpReceiver()
	}
	sort.Strings(metricReceivers)
	config := CollectorConfig{
		Extensions: map[string]any{"health_check": map[string]any{"endpoint": DefaultHealthEndpoint}}, Receivers: receivers,
		Processors: map[string]any{"memory_limiter": map[string]any{"check_interval": "1s", "limit_percentage": 75, "spike_limit_percentage": 15}, oksgSourceTaggerProcessor: sourceTagger(requirements.SourceStamps), "batch": map[string]any{}},
		Connectors: connectors, Exporters: map[string]any{otlpGRPCExporter: gatewayExporter()},
		Service: ServiceConfig{Extensions: []string{"health_check"}, Pipelines: map[string]PipelineConfig{
			"metrics":      {Receivers: metricReceivers, Processors: []string{"memory_limiter", oksgSourceTaggerProcessor, "batch"}, Exporters: []string{otlpGRPCExporter}},
			"metrics/otlp": {Receivers: []string{"otlp"}, Processors: []string{"memory_limiter", "batch"}, Exporters: []string{otlpGRPCExporter}},
		}},
	}
	if requirements.OTLP.Traces {
		exporters := []string{otlpGRPCExporter}
		if requirements.ServiceGraph != nil {
			exporters = append(exporters, "service_graph")
		}
		config.Service.Pipelines["traces"] = PipelineConfig{Receivers: []string{"otlp"}, Processors: []string{"memory_limiter", "batch"}, Exporters: exporters}
	}
	return config
}

func otlpReceiver() map[string]any {
	return map[string]any{"protocols": map[string]any{"grpc": map[string]any{"endpoint": "0.0.0.0:4317"}, "http": map[string]any{"endpoint": DefaultGatewayHTTPEndpoint}}}
}
func agentExporter() map[string]any {
	return map[string]any{"endpoint": DefaultGatewayGRPCEndpoint, "tls": map[string]any{"insecure": true}}
}
func gatewayExporter() map[string]any {
	return map[string]any{"endpoint": DefaultBackendGRPCEndpoint, "tls": map[string]any{"insecure": true}, "sending_queue": map[string]any{"enabled": true}, "retry_on_failure": map[string]any{"enabled": true}}
}

func prometheusReceiver(requirement *collectorspec.Prometheus) map[string]any {
	scrapes := make([]map[string]any, 0, len(requirement.Targets))
	for _, target := range requirement.Targets {
		scrapes = append(scrapes, map[string]any{"job_name": fmt.Sprintf("prometheus-app:%s:%s:%d", target.Namespace, target.ServiceName, target.Port), "metrics_path": target.Path, "static_configs": []map[string]any{{"targets": []string{fmt.Sprintf("%s.%s.svc.cluster.local:%d", target.ServiceName, target.Namespace, target.Port)}, "labels": map[string]string{"service": target.ServiceName, "namespace": target.Namespace, "metrics_path": target.Path, "service_port": fmt.Sprintf("%d", target.Port)}}}, "metric_relabel_configs": []map[string]any{{"target_label": "oksg_source", "replacement": requirement.SourceID}}})
	}
	return map[string]any{"config": map[string]any{"scrape_configs": scrapes}}
}

func istioReceiver(requirement *collectorspec.Istio) map[string]any {
	flavor := requirement.Flavor
	if flavor == "" {
		flavor = "istio"
	}
	relabel := []map[string]any{{"source_labels": []string{"__meta_kubernetes_pod_container_name"}, "action": "keep", "regex": "istio-proxy"}, {"source_labels": []string{"__meta_kubernetes_pod_node_name"}, "action": "keep", "regex": `^\Q${env:K8S_NODE_NAME}\E$`}, {"source_labels": []string{"__address__"}, "target_label": "__address__", "regex": "([^:]+)(?::\\d+)?", "replacement": fmt.Sprintf("$1:%d", requirement.MetricsPort)}}
	if len(requirement.InjectedNamespaces) > 0 {
		relabel = append(relabel, map[string]any{"source_labels": []string{"__meta_kubernetes_namespace"}, "action": "keep", "regex": strings.Join(requirement.InjectedNamespaces, "|")})
	}
	return map[string]any{"config": map[string]any{"scrape_configs": []map[string]any{{"job_name": "mesh-" + flavor, "scrape_interval": "15s", "kubernetes_sd_configs": []map[string]any{{"role": "pod"}}, "metrics_path": requirement.MetricsPath, "relabel_configs": relabel, "metric_relabel_configs": []map[string]any{{"source_labels": []string{"__name__"}, "action": "keep", "regex": "istio_(requests_total|request_duration_milliseconds(_bucket|_sum|_count)?|request_messages_total|response_messages_total|tcp_connections_(opened|closed)_total|tcp_(sent|received)_bytes_total)"}, {"source_labels": []string{"reporter"}, "action": "keep", "regex": "destination"}, {"target_label": "oksg_source", "replacement": requirement.SourceID}, {"source_labels": []string{"source_canonical_service"}, "target_label": "client"}, {"source_labels": []string{"source_workload_namespace"}, "target_label": "client_namespace"}, {"source_labels": []string{"destination_canonical_service"}, "target_label": "server"}, {"source_labels": []string{"destination_service_namespace"}, "target_label": "server_namespace"}, {"source_labels": []string{"request_protocol"}, "target_label": "network_protocol_name"}, {"source_labels": []string{"response_code"}, "target_label": "http_response_status_code"}, {"source_labels": []string{"grpc_response_status"}, "target_label": "rpc_grpc_status_code"}, {"source_labels": []string{"connection_security_policy"}, "target_label": "mesh_security_policy"}, {"source_labels": []string{"source_principal"}, "target_label": "mesh_peer_source_principal"}, {"source_labels": []string{"destination_principal"}, "target_label": "mesh_peer_destination_principal"}, {"source_labels": []string{"response_flags"}, "target_label": "mesh_envoy_response_flags"}}}}}}
}

func sourceTagger(stamps []collectorspec.SourceStamp) map[string]any {
	statements := make([]string, 0, len(stamps))
	for _, stamp := range stamps {
		statements = append(statements, sourceStampStatement(stamp))
	}
	return map[string]any{"metric_statements": []map[string]any{{"context": "datapoint", "statements": statements}}}
}

func sourceStampStatement(stamp collectorspec.SourceStamp) string {
	conditions := make([]string, 0, len(stamp.ExactMetricNames)+len(stamp.MetricPatterns))
	for _, name := range stamp.ExactMetricNames {
		conditions = append(conditions, fmt.Sprintf("metric.name == %q", name))
	}
	for _, pattern := range stamp.MetricPatterns {
		conditions = append(conditions, fmt.Sprintf("IsMatch(metric.name, %q)", pattern))
	}
	condition := "false"
	if len(conditions) > 0 {
		condition = "(" + strings.Join(conditions, " or ") + ")"
	}
	if stamp.OnlyIfUnset {
		condition += ` and datapoint.attributes["oksg_source"] == nil`
	}
	return fmt.Sprintf(`set(datapoint.attributes["oksg_source"], %q) where %s`, stamp.SourceID, condition)
}
