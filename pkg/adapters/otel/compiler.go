// Package otel translates provider-neutral collection requirements into
// OpenTelemetry Collector configuration.
package otel

import (
	"fmt"
	"sort"
	"strings"

	collectorspec "github.com/erickivel/karthos/pkg/collector/spec"
)

const (
	DefaultGatewayHTTPEndpoint     = "0.0.0.0:4318"
	DefaultHealthEndpoint          = "0.0.0.0:13133"
	backendGRPCEndpointEnv         = "${env:KARTHOS_BACKEND_GRPC_ENDPOINT}"
	gatewayGRPCEndpointEnv         = "${env:KARTHOS_GATEWAY_GRPC_ENDPOINT}"
	otlpGRPCExporter               = "otlp_grpc"
	karthosSourceTaggerProcessor   = "transform/karthos-source"
	serviceGraphNamespaceProcessor = "transform/karthos-servicegraph-namespace"
	collectorDebugExporter         = "debug/karthos"
)

// kubeClusterV0156MetricDefaults is the complete metric inventory exposed by
// the deployed k8s_cluster receiver version. Writing every entry explicitly
// prevents receiver defaults from bypassing the source module's PoC scope.
var kubeClusterV0156MetricDefaults = []string{
	"k8s.container.cpu_limit", "k8s.container.cpu_request", "k8s.container.ephemeralstorage_limit", "k8s.container.ephemeralstorage_request",
	"k8s.container.memory_limit", "k8s.container.memory_request", "k8s.container.ready", "k8s.container.restarts", "k8s.container.status.reason",
	"k8s.container.status.state", "k8s.container.storage_limit", "k8s.container.storage_request", "k8s.cronjob.active_jobs",
	"k8s.daemonset.current_scheduled_nodes", "k8s.daemonset.desired_scheduled_nodes", "k8s.daemonset.misscheduled_nodes", "k8s.daemonset.ready_nodes",
	"k8s.deployment.available", "k8s.deployment.desired", "k8s.hpa.current_replicas", "k8s.hpa.desired_replicas", "k8s.hpa.max_replicas", "k8s.hpa.min_replicas",
	"k8s.job.active_pods", "k8s.job.desired_successful_pods", "k8s.job.failed_pods", "k8s.job.max_parallel_pods", "k8s.job.successful_pods", "k8s.namespace.phase",
	"k8s.node.condition", "k8s.persistentvolume.status.phase", "k8s.persistentvolume.storage.capacity", "k8s.persistentvolumeclaim.status.phase",
	"k8s.persistentvolumeclaim.storage.capacity", "k8s.persistentvolumeclaim.storage.request", "k8s.pod.phase", "k8s.pod.status_reason", "k8s.replicaset.available",
	"k8s.replicaset.desired", "k8s.replication_controller.available", "k8s.replication_controller.desired", "k8s.resource_quota.hard_limit", "k8s.resource_quota.used",
	"k8s.service.endpoint.count", "k8s.service.load_balancer.ingress.count", "k8s.statefulset.current_pods", "k8s.statefulset.desired_pods", "k8s.statefulset.ready_pods",
	"k8s.statefulset.updated_pods", "openshift.appliedclusterquota.limit", "openshift.appliedclusterquota.used", "openshift.clusterquota.limit", "openshift.clusterquota.used",
}

type BackendConfig struct {
	Enabled bool
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
	Telemetry  ServiceTelemetryConfig    `json:"telemetry" yaml:"telemetry"`
}

// ServiceTelemetryConfig is the fixed Collector self-telemetry policy. It is
// intentionally owned by this adapter rather than exposed as a chart setting.
type ServiceTelemetryConfig struct {
	Logs ServiceTelemetryLogs `json:"logs" yaml:"logs"`
}

type ServiceTelemetryLogs struct {
	Encoding    string   `json:"encoding" yaml:"encoding"`
	Level       string   `json:"level" yaml:"level"`
	OutputPaths []string `json:"output_paths" yaml:"output_paths"`
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
	AgentConfig    CollectorConfig
	GatewayConfig  CollectorConfig
}

// CompilerOptions are fixed OpenTelemetry adapter choices supplied by the
// deployment command. They do not expose Collector configuration syntax.
type CompilerOptions struct {
	DebugEnabled bool
}

type Compiler struct {
	options CompilerOptions
}

func NewCompiler() *Compiler { return &Compiler{} }

func NewCompilerWithOptions(options CompilerOptions) *Compiler {
	return &Compiler{options: options}
}

func (c *Compiler) Compile(plan collectorspec.CollectionPlan) (Deployment, error) {
	if plan.Namespace == "" {
		return Deployment{}, fmt.Errorf("collection plan namespace is required")
	}
	if plan.ClusterName == "" {
		return Deployment{}, fmt.Errorf("collection plan cluster name is required")
	}
	if err := plan.Normalize(); err != nil {
		return Deployment{}, fmt.Errorf("normalize collection plan: %w", err)
	}
	agentConfig := compileAgent(plan.Agent)
	gatewayConfig := compileGateway(plan.Gateway)
	if c.options.DebugEnabled {
		attachCollectorDebug(&agentConfig)
		attachCollectorDebug(&gatewayConfig)
	}
	return Deployment{
		ClusterName: plan.ClusterName, Namespace: plan.Namespace,
		EnabledSignals: append([]string(nil), plan.EnabledSignals...),
		Backend:        BackendConfig{Enabled: true},
		AgentConfig:    agentConfig, GatewayConfig: gatewayConfig,
	}, nil
}

func attachCollectorDebug(config *CollectorConfig) {
	config.Exporters[collectorDebugExporter] = map[string]any{
		"verbosity":           "detailed",
		"use_internal_logger": false,
		"output_paths":        []string{"stdout"},
	}
	for pipelineName, pipeline := range config.Service.Pipelines {
		pipeline.Exporters = append(pipeline.Exporters, collectorDebugExporter)
		config.Service.Pipelines[pipelineName] = pipeline
	}
}

func fixedServiceTelemetry() ServiceTelemetryConfig {
	return ServiceTelemetryConfig{Logs: ServiceTelemetryLogs{
		Encoding:    "json",
		Level:       "info",
		OutputPaths: []string{"stdout"},
	}}
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
			"resource":                   map[string]any{"attributes": []map[string]any{{"key": "k8s.node.name", "value": "${env:K8S_NODE_NAME}", "action": "insert"}}},
			"k8s_attributes":             map[string]any{"auth_type": "serviceAccount", "passthrough": false},
			karthosSourceTaggerProcessor: sourceTagger(requirements.SourceStamps), "batch": map[string]any{},
		},
		Exporters: map[string]any{otlpGRPCExporter: agentExporter()},
		Service: ServiceConfig{Extensions: []string{"health_check"}, Telemetry: fixedServiceTelemetry(), Pipelines: map[string]PipelineConfig{
			"metrics": {Receivers: metricReceivers, Processors: []string{"resource", "k8s_attributes", karthosSourceTaggerProcessor, "batch"}, Exporters: []string{otlpGRPCExporter}},
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
		for _, name := range kubeClusterV0156MetricDefaults {
			metricsEnabled[name] = map[string]any{"enabled": false}
		}
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
		Processors: map[string]any{"memory_limiter": map[string]any{"check_interval": "1s", "limit_percentage": 75, "spike_limit_percentage": 15}, karthosSourceTaggerProcessor: sourceTagger(requirements.SourceStamps), "batch": map[string]any{}},
		Connectors: connectors, Exporters: map[string]any{otlpGRPCExporter: gatewayExporter()},
		Service: ServiceConfig{Extensions: []string{"health_check"}, Telemetry: fixedServiceTelemetry(), Pipelines: map[string]PipelineConfig{
			"metrics":      {Receivers: metricReceivers, Processors: []string{"memory_limiter", karthosSourceTaggerProcessor, "batch"}, Exporters: []string{otlpGRPCExporter}},
			"metrics/otlp": {Receivers: []string{"otlp"}, Processors: []string{"memory_limiter", "batch"}, Exporters: []string{otlpGRPCExporter}},
		}},
	}
	if requirements.OTLP.Traces {
		exporters := []string{otlpGRPCExporter}
		processors := []string{"memory_limiter", "batch"}
		if requirements.ServiceGraph != nil {
			exporters = append(exporters, "service_graph")
			config.Processors[serviceGraphNamespaceProcessor] = map[string]any{
				"trace_statements": []any{map[string]any{
					"context":    "span",
					"statements": []string{`set(span.attributes["k8s.namespace.name"], resource.attributes["k8s.namespace.name"]) where span.attributes["k8s.namespace.name"] == nil and resource.attributes["k8s.namespace.name"] != nil`},
				}},
			}
			processors = []string{"memory_limiter", serviceGraphNamespaceProcessor, "batch"}
		}
		config.Service.Pipelines["traces"] = PipelineConfig{Receivers: []string{"otlp"}, Processors: processors, Exporters: exporters}
	}
	return config
}

func otlpReceiver() map[string]any {
	return map[string]any{"protocols": map[string]any{"grpc": map[string]any{"endpoint": "0.0.0.0:4317"}, "http": map[string]any{"endpoint": DefaultGatewayHTTPEndpoint}}}
}
func agentExporter() map[string]any {
	return map[string]any{"endpoint": gatewayGRPCEndpointEnv, "tls": map[string]any{"insecure": true}}
}
func gatewayExporter() map[string]any {
	return map[string]any{"endpoint": backendGRPCEndpointEnv, "tls": map[string]any{"insecure": true}, "sending_queue": map[string]any{"enabled": true}, "retry_on_failure": map[string]any{"enabled": true}}
}

func prometheusReceiver(requirement *collectorspec.Prometheus) map[string]any {
	scrapes := make([]map[string]any, 0, len(requirement.Targets))
	for _, target := range requirement.Targets {
		scrapes = append(scrapes, map[string]any{"job_name": fmt.Sprintf("prometheus-app:%s:%s:%d", target.Namespace, target.ServiceName, target.Port), "scrape_interval": "15s", "metrics_path": target.Path, "static_configs": []map[string]any{{"targets": []string{fmt.Sprintf("%s.%s.svc.cluster.local:%d", target.ServiceName, target.Namespace, target.Port)}, "labels": map[string]string{"service": target.ServiceName, "namespace": target.Namespace, "metrics_path": target.Path, "service_port": fmt.Sprintf("%d", target.Port)}}}, "metric_relabel_configs": []map[string]any{{"target_label": "karthos_source", "replacement": requirement.SourceID}}})
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
	return map[string]any{"config": map[string]any{"scrape_configs": []map[string]any{{"job_name": "mesh-" + flavor, "scrape_interval": "15s", "kubernetes_sd_configs": []map[string]any{{"role": "pod"}}, "metrics_path": requirement.MetricsPath, "relabel_configs": relabel, "metric_relabel_configs": []map[string]any{{"source_labels": []string{"__name__"}, "action": "keep", "regex": "istio_(requests_total|request_duration_milliseconds(_bucket|_sum|_count)?|request_messages_total|response_messages_total|tcp_connections_(opened|closed)_total|tcp_(sent|received)_bytes_total)"}, {"source_labels": []string{"reporter"}, "action": "keep", "regex": "destination"}, {"target_label": "karthos_source", "replacement": requirement.SourceID}, {"source_labels": []string{"source_canonical_service"}, "target_label": "client"}, {"source_labels": []string{"source_workload_namespace"}, "target_label": "client_namespace"}, {"source_labels": []string{"destination_canonical_service"}, "target_label": "server"}, {"source_labels": []string{"destination_service_namespace"}, "target_label": "server_namespace"}, {"source_labels": []string{"request_protocol"}, "target_label": "network_protocol_name"}, {"source_labels": []string{"response_code"}, "target_label": "http_response_status_code"}, {"source_labels": []string{"grpc_response_status"}, "target_label": "rpc_grpc_status_code"}, {"source_labels": []string{"connection_security_policy"}, "target_label": "mesh_security_policy"}, {"source_labels": []string{"source_principal"}, "target_label": "mesh_peer_source_principal"}, {"source_labels": []string{"destination_principal"}, "target_label": "mesh_peer_destination_principal"}, {"source_labels": []string{"response_flags"}, "target_label": "mesh_envoy_response_flags"}}}}}}
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
		condition += ` and datapoint.attributes["karthos_source"] == nil`
	}
	return fmt.Sprintf(`set(datapoint.attributes["karthos_source"], %q) where %s`, stamp.SourceID, condition)
}
