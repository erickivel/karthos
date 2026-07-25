package otel

import (
	"context"
	"fmt"

	"github.com/erickivel/oksg/pkg/core/metrics"
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
	otlpGRPCExporter           = "otlp_grpc"
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

	_, selected, err := metrics.Resolve(metrics.DefaultCatalog(), metrics.DefaultLogicalSources(), clusterProfile)
	if err != nil {
		return nil, fmt.Errorf("resolve sources: %w", err)
	}

	enabledSignals := []string{"metric"}
	agentReceivers := map[string]any{
		"kubelet_stats": map[string]any{
			"collection_interval":  "30s",
			"auth_type":            "serviceAccount",
			"endpoint":             "https://${env:K8S_NODE_NAME}:10250",
			"insecure_skip_verify": true,
			"metric_groups":        []string{"node", "pod", "container", "volume"},
			"collect_all_network_interfaces": map[string]any{
				"node": true,
				"pod":  false,
			},
			"extra_metadata_labels": []string{"k8s.volume.type"},
		},
		"host_metrics": map[string]any{
			"collection_interval": "30s",
			"root_path":           "/hostfs",
			"scrapers": map[string]any{
				"cpu": map[string]any{
					"metrics": map[string]any{
						"system.cpu.utilization": map[string]any{"enabled": true},
					},
				},
				"memory": map[string]any{},
				"disk":   map[string]any{},
				"filesystem": map[string]any{
					"metrics": map[string]any{
						"system.filesystem.utilization": map[string]any{"enabled": true},
					},
				},
				"network": map[string]any{},
				"load":    map[string]any{},
				"paging":  map[string]any{},
			},
		},
	}

	pipelineReceivers := []string{"kubelet_stats", "host_metrics"}
	var scrapeConfigs []map[string]any
	if selected.Has(metrics.SourceMeshIstio) {
		if mesh := meshSource(clusterProfile.Sources); mesh != nil {
			scrapeConfigs = append(scrapeConfigs, buildIstioScrapeConfig(mesh))
		}
	}
	if len(scrapeConfigs) > 0 {
		agentReceivers["prometheus"] = map[string]any{
			"config": map[string]any{
				"scrape_configs": scrapeConfigs,
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
			oksgSourceTaggerProcessor: buildOksgSourceTagger(),
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
			"allocatable_types_to_report": []string{"cpu", "memory", "ephemeral-storage", "pods"},
			"metrics": map[string]any{
				"k8s.container.storage_limit": map[string]any{
					"enabled": false,
				},
				"k8s.container.storage_request": map[string]any{
					"enabled": false,
				},
				"k8s.node.condition": map[string]any{
					"enabled": true,
				},
				"k8s.container.ready": map[string]any{
					"enabled": true,
				},
				"k8s.container.restarts": map[string]any{
					"enabled": true,
				},
				// Default-disabled in v0.156; surfaces the integer-encoded
				// pod failure reason (Evicted, NodeAffinity, etc.) used by
				// the derived oksg.pod.health.status enum.
				"k8s.pod.status_reason": map[string]any{
					"enabled": true,
				},
			},
		},
	}
	gatewayMetricsReceivers := []string{"k8s_cluster"}
	if selected.Has(metrics.SourcePrometheusApp) {
		gatewayReceivers["prometheus"] = map[string]any{
			"config": map[string]any{
				"scrape_configs": buildPrometheusScrapeConfigs(prometheusTargets(clusterProfile.Sources)),
			},
		}
		gatewayMetricsReceivers = append(gatewayMetricsReceivers, "prometheus")
	}
	var gatewayConnectors map[string]any
	deploySpanmetrics := selected.Has(metrics.SourceTracesSpanmetrics)
	deployServicegraph := selected.Has(metrics.SourceTracesServicegraph)
	if deploySpanmetrics || deployServicegraph {
		gatewayConnectors = map[string]any{}
		if deploySpanmetrics {
			gatewayConnectors["span_metrics"] = map[string]any{
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
			}
			gatewayMetricsReceivers = append(gatewayMetricsReceivers, "span_metrics")
		}
		if deployServicegraph {
			gatewayConnectors["service_graph"] = map[string]any{
				"metrics_flush_interval":    "15s",
				"latency_histogram_buckets": []string{"100ms", "250ms", "500ms", "1s", "5s"},
				"dimensions":                []string{"http.method"},
				"store": map[string]any{
					"ttl":       "30s",
					"max_items": 1000,
				},
			}
			gatewayMetricsReceivers = append(gatewayMetricsReceivers, "service_graph")
		}
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
			oksgSourceTaggerProcessor: buildOksgSourceTagger(),
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
		if deploySpanmetrics {
			traceExporters = append(traceExporters, "span_metrics")
		}
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
			"job_name":     fmt.Sprintf("prometheus-app:%s:%s:%d", target.Namespace, target.ServiceName, target.Port),
			"metrics_path": target.Path,
			"static_configs": []map[string]any{
				{
					"targets": []string{
						fmt.Sprintf("%s.%s.svc.cluster.local:%d", target.ServiceName, target.Namespace, target.Port),
					},
					"labels": map[string]string{
						"service":      target.ServiceName,
						"namespace":    target.Namespace,
						"metrics_path": target.Path,
						"service_port": fmt.Sprintf("%d", target.Port),
					},
				},
			},
			"metric_relabel_configs": []map[string]any{
				{
					"target_label": "oksg_source",
					"replacement":  metrics.SourcePrometheusApp,
				},
			},
		})
	}
	return scrapeConfigs
}

func meshSource(sources []profile.ObservabilitySource) *profile.MeshSource {
	for _, source := range sources {
		if source.Type == "istio" && source.Present && source.Mesh != nil {
			return source.Mesh
		}
	}
	return nil
}

func buildIstioScrapeConfig(mesh *profile.MeshSource) map[string]any {
	port := mesh.MetricsPort
	if port == 0 {
		port = 15020
	}
	path := mesh.MetricsPath
	if path == "" {
		path = "/stats/prometheus"
	}

	relabelConfigs := []map[string]any{
		{
			"source_labels": []string{"__meta_kubernetes_pod_container_name"},
			"action":        "keep",
			"regex":         "istio-proxy",
		},
		{
			"source_labels": []string{"__meta_kubernetes_pod_node_name"},
			"action":        "keep",
			"regex":         "${env:K8S_NODE_NAME}",
		},
		{
			"source_labels": []string{"__address__"},
			"target_label":  "__address__",
			"regex":         "([^:]+)(?::\\d+)?",
			"replacement":   fmt.Sprintf("$1:%d", port),
		},
	}
	if len(mesh.InjectedNamespaces) > 0 {
		relabelConfigs = append(relabelConfigs, map[string]any{
			"source_labels": []string{"__meta_kubernetes_namespace"},
			"action":        "keep",
			"regex":         joinRegex(mesh.InjectedNamespaces),
		})
	}

	return map[string]any{
		"job_name":        pathlessJobName(mesh.Flavor),
		"scrape_interval": "15s",
		"kubernetes_sd_configs": []map[string]any{
			{"role": "pod"},
		},
		"metrics_path":    path,
		"relabel_configs": relabelConfigs,
		"metric_relabel_configs": []map[string]any{
			{
				"source_labels": []string{"__name__"},
				"action":        "keep",
				"regex":         "istio_(requests_total|request_duration_milliseconds(_bucket|_sum|_count)?|request_bytes(_bucket|_sum|_count)?|response_bytes(_bucket|_sum|_count)?|request_messages_total|response_messages_total|tcp_sent_bytes_total|tcp_received_bytes_total|tcp_connections_opened_total|tcp_connections_closed_total)",
			},
			{
				"source_labels": []string{"reporter"},
				"action":        "keep",
				"regex":         "destination",
			},
			{
				"target_label": "oksg_source",
				"replacement":  "mesh.istio",
			},
			{
				"source_labels": []string{"source_canonical_service"},
				"target_label":  "client",
			},
			{
				"source_labels": []string{"source_workload_namespace"},
				"target_label":  "client_namespace",
			},
			{
				"source_labels": []string{"destination_canonical_service"},
				"target_label":  "server",
			},
			{
				"source_labels": []string{"destination_service_namespace"},
				"target_label":  "server_namespace",
			},
			{
				"source_labels": []string{"request_protocol"},
				"target_label":  "network_protocol_name",
			},
			{
				"source_labels": []string{"response_code"},
				"target_label":  "http_response_status_code",
			},
			{
				"source_labels": []string{"grpc_response_status"},
				"target_label":  "rpc_grpc_status_code",
			},
			{
				"source_labels": []string{"connection_security_policy"},
				"target_label":  "mesh_security_policy",
			},
			{
				"source_labels": []string{"source_principal"},
				"target_label":  "mesh_peer_source_principal",
			},
			{
				"source_labels": []string{"destination_principal"},
				"target_label":  "mesh_peer_destination_principal",
			},
			{
				"source_labels": []string{"response_flags"},
				"target_label":  "mesh_envoy_response_flags",
			},
		},
	}
}

func pathlessJobName(flavor string) string {
	if flavor == "" {
		flavor = "istio"
	}
	return "mesh-" + flavor
}

func joinRegex(values []string) string {
	if len(values) == 0 {
		return ""
	}
	out := values[0]
	for _, value := range values[1:] {
		out += "|" + value
	}
	return out
}

// oksgSourceTaggerProcessor names the OTel transform processor that stamps
// `oksg_source` onto each datapoint based on the receiver-determined metric
// name space. The istio prometheus scrape already sets `oksg_source` via
// metric_relabel_configs; the where clauses below skip overwriting that.
const oksgSourceTaggerProcessor = "transform/oksg-source"

// buildOksgSourceTagger emits OTTL statements for collector-owned receiver and
// connector pipelines. Those components own disjoint wire-name namespaces, so
// name matching is deterministic. External OTLP metrics are isolated in
// metrics/otlp and never run this processor. Any new collector-owned component
// added to the planner must extend this tagger.
func buildOksgSourceTagger() map[string]any {
	guard := `datapoint.attributes["oksg_source"] == nil`
	statements := []string{
		// kubelet_stats receiver. The pod-scope rule lists only the metric
		// families kubelet_stats actually emits (cpu, memory, filesystem,
		// network); pod-scope status metrics like k8s.pod.phase /
		// status_reason originate from k8s_cluster and are tagged kube-cluster
		// below. Volume metrics live at top-level k8s.volume.* and are
		// matched by the next clause. Container CPU/memory usage from
		// kubelet_stats arrives on the wire as raw `container.*` (translated
		// to `k8s.container.*` only at the backend dialect step), so
		// `k8s.container.*` here at the gateway/agent always originates from
		// the k8s_cluster receiver.
		`set(datapoint.attributes["oksg_source"], "` + metrics.SourceKubelet + `") where IsMatch(metric.name, "^k8s\\.pod\\.(cpu|memory|filesystem|network)\\.") and ` + guard,
		`set(datapoint.attributes["oksg_source"], "` + metrics.SourceKubelet + `") where IsMatch(metric.name, "^k8s\\.volume\\.") and ` + guard,
		`set(datapoint.attributes["oksg_source"], "` + metrics.SourceKubelet + `") where IsMatch(metric.name, "^container\\.") and ` + guard,
		`set(datapoint.attributes["oksg_source"], "` + metrics.SourceKubelet + `") where (metric.name == "k8s.node.cpu.usage" or metric.name == "k8s.node.memory.working_set" or IsMatch(metric.name, "^k8s\\.node\\.(network|filesystem)\\.")) and ` + guard,
		// host_metrics receiver
		`set(datapoint.attributes["oksg_source"], "` + metrics.SourceHostOS + `") where IsMatch(metric.name, "^system\\.") and ` + guard,
		// k8s_cluster receiver: cluster-level workload + capacity + condition + container/pod spec/status metrics.
		// The broad ^k8s\.pod\. rule is intentionally last among the pod
		// matchers — it catches pod.phase / pod.status_reason and any future
		// pod-scope k8s_cluster signal that the kubelet rules above didn't
		// claim.
		`set(datapoint.attributes["oksg_source"], "` + metrics.SourceKubeCluster + `") where IsMatch(metric.name, "^k8s\\.container\\.") and ` + guard,
		`set(datapoint.attributes["oksg_source"], "` + metrics.SourceKubeCluster + `") where IsMatch(metric.name, "^k8s\\.pod\\.") and ` + guard,
		`set(datapoint.attributes["oksg_source"], "` + metrics.SourceKubeCluster + `") where IsMatch(metric.name, "^k8s\\.(replicaset|deployment|namespace|daemonset|statefulset|job|cronjob|hpa|resource_quota|replication_controller)\\.") and ` + guard,
		`set(datapoint.attributes["oksg_source"], "` + metrics.SourceKubeCluster + `") where IsMatch(metric.name, "^k8s\\.node\\.(condition|allocatable|cpu\\.capacity|cpu\\.cores|cpu\\.frequency|memory\\.capacity|disk\\.capacity|network\\.bandwidth)") and ` + guard,
		// span_metrics connector
		`set(datapoint.attributes["oksg_source"], "` + metrics.SourceTracesSpanmetrics + `") where IsMatch(metric.name, "^traces\\.span\\.metrics\\.") and ` + guard,
		// service_graph connector
		`set(datapoint.attributes["oksg_source"], "` + metrics.SourceTracesServicegraph + `") where IsMatch(metric.name, "^traces_service_graph_") and ` + guard,
		`set(datapoint.attributes["oksg_source"], "` + metrics.SourceTracesServicegraph + `") where IsMatch(metric.name, "^traces\\.service\\.graph\\.") and ` + guard,
	}
	return map[string]any{
		"metric_statements": []map[string]any{
			{
				"context":    "datapoint",
				"statements": statements,
			},
		},
	}
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
