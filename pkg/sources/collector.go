package sources

import (
	"fmt"
	"strings"

	"github.com/erickivel/oksg/pkg/discovery/profile"
)

func contributeKubelet(out *CollectorContribution, _ *profile.ClusterProfile) {
	out.AgentReceivers["kubelet_stats"] = map[string]any{
		"collection_interval":   "30s",
		"auth_type":             "serviceAccount",
		"endpoint":              "https://${env:K8S_NODE_NAME}:10250",
		"insecure_skip_verify":  true,
		"metric_groups":         []string{"node", "pod", "container", "volume"},
		"extra_metadata_labels": []string{"k8s.volume.type"},
	}
	out.AgentMetricReceivers = append(out.AgentMetricReceivers, "kubelet_stats")
	guard := `datapoint.attributes["oksg_source"] == nil`
	out.AgentSourceStamps = append(out.AgentSourceStamps,
		`set(datapoint.attributes["oksg_source"], "`+IDKubelet+`") where IsMatch(metric.name, "^k8s\\.pod\\.(cpu|memory)\\.") and `+guard,
		`set(datapoint.attributes["oksg_source"], "`+IDKubelet+`") where IsMatch(metric.name, "^k8s\\.volume\\.") and `+guard,
		`set(datapoint.attributes["oksg_source"], "`+IDKubelet+`") where IsMatch(metric.name, "^container\\.") and `+guard,
		`set(datapoint.attributes["oksg_source"], "`+IDKubelet+`") where (metric.name == "k8s.node.cpu.usage" or metric.name == "k8s.node.memory.working_set") and `+guard,
	)
}

func contributeHostOS(out *CollectorContribution, _ *profile.ClusterProfile) {
	out.AgentReceivers["host_metrics"] = map[string]any{
		"collection_interval": "30s", "root_path": "/hostfs",
		"scrapers": map[string]any{
			"cpu":    map[string]any{"metrics": map[string]any{"system.cpu.utilization": map[string]any{"enabled": true}}},
			"memory": map[string]any{}, "disk": map[string]any{},
			"filesystem": map[string]any{"metrics": map[string]any{"system.filesystem.utilization": map[string]any{"enabled": true}}},
			"network":    map[string]any{},
		},
	}
	out.AgentMetricReceivers = append(out.AgentMetricReceivers, "host_metrics")
	out.AgentSourceStamps = append(out.AgentSourceStamps,
		`set(datapoint.attributes["oksg_source"], "`+IDHostOS+`") where IsMatch(metric.name, "^system\\.") and datapoint.attributes["oksg_source"] == nil`,
	)
}

func contributeKubeCluster(out *CollectorContribution, _ *profile.ClusterProfile) {
	out.GatewayReceivers["k8s_cluster"] = map[string]any{
		"collection_interval": "30s", "node_conditions_to_report": []string{"Ready", "MemoryPressure", "DiskPressure"},
		"metrics": map[string]any{
			"k8s.node.condition": map[string]any{"enabled": true}, "k8s.container.ready": map[string]any{"enabled": true},
			"k8s.container.restarts": map[string]any{"enabled": true}, "k8s.pod.status_reason": map[string]any{"enabled": true},
		},
	}
	out.GatewayMetricReceivers = append(out.GatewayMetricReceivers, "k8s_cluster")
	guard := `datapoint.attributes["oksg_source"] == nil`
	out.GatewaySourceStamps = append(out.GatewaySourceStamps,
		`set(datapoint.attributes["oksg_source"], "`+IDKubeCluster+`") where IsMatch(metric.name, "^k8s\\.container\\.(cpu|memory)_(request|limit)$") and `+guard,
		`set(datapoint.attributes["oksg_source"], "`+IDKubeCluster+`") where (metric.name == "k8s.container.ready" or metric.name == "k8s.container.restarts") and `+guard,
		`set(datapoint.attributes["oksg_source"], "`+IDKubeCluster+`") where (metric.name == "k8s.pod.phase" or metric.name == "k8s.pod.status_reason") and `+guard,
		`set(datapoint.attributes["oksg_source"], "`+IDKubeCluster+`") where IsMatch(metric.name, "^k8s\\.node\\.condition\\.(ready|memory_pressure|disk_pressure)$") and `+guard,
	)
}

func contributePrometheusApp(out *CollectorContribution, capabilityProfile *profile.ClusterProfile) {
	targets := capabilityProfile.PrometheusTargets()
	if len(targets) == 0 {
		return
	}
	scrapes := make([]map[string]any, 0, len(targets))
	for _, target := range targets {
		scrapes = append(scrapes, map[string]any{
			"job_name": fmt.Sprintf("prometheus-app:%s:%s:%d", target.Namespace, target.ServiceName, target.Port), "metrics_path": target.Path,
			"static_configs":         []map[string]any{{"targets": []string{fmt.Sprintf("%s.%s.svc.cluster.local:%d", target.ServiceName, target.Namespace, target.Port)}, "labels": map[string]string{"service": target.ServiceName, "namespace": target.Namespace, "metrics_path": target.Path, "service_port": fmt.Sprintf("%d", target.Port)}}},
			"metric_relabel_configs": []map[string]any{{"target_label": "oksg_source", "replacement": IDPrometheusApp}},
		})
	}
	out.GatewayReceivers["prometheus"] = map[string]any{"config": map[string]any{"scrape_configs": scrapes}}
	out.GatewayMetricReceivers = append(out.GatewayMetricReceivers, "prometheus")
}

func contributeMeshIstio(out *CollectorContribution, capabilityProfile *profile.ClusterProfile) {
	mesh := capabilityProfile.Mesh()
	if mesh == nil {
		return
	}
	port, path := mesh.MetricsPort, mesh.MetricsPath
	if port == 0 {
		port = 15020
	}
	if path == "" {
		path = "/stats/prometheus"
	}
	relabel := []map[string]any{{"source_labels": []string{"__meta_kubernetes_pod_container_name"}, "action": "keep", "regex": "istio-proxy"}, {"source_labels": []string{"__meta_kubernetes_pod_node_name"}, "action": "keep", "regex": `^\Q${env:K8S_NODE_NAME}\E$`}, {"source_labels": []string{"__address__"}, "target_label": "__address__", "regex": "([^:]+)(?::\\d+)?", "replacement": fmt.Sprintf("$1:%d", port)}}
	if len(mesh.InjectedNamespaces) > 0 {
		relabel = append(relabel, map[string]any{"source_labels": []string{"__meta_kubernetes_namespace"}, "action": "keep", "regex": strings.Join(mesh.InjectedNamespaces, "|")})
	}
	out.AgentReceivers["prometheus"] = map[string]any{"config": map[string]any{"scrape_configs": []map[string]any{{
		"job_name": "mesh-" + defaultString(mesh.Flavor, "istio"), "scrape_interval": "15s", "kubernetes_sd_configs": []map[string]any{{"role": "pod"}}, "metrics_path": path, "relabel_configs": relabel,
		"metric_relabel_configs": []map[string]any{{"source_labels": []string{"__name__"}, "action": "keep", "regex": "istio_(requests_total|request_duration_milliseconds(_bucket|_sum|_count)?)"}, {"source_labels": []string{"reporter"}, "action": "keep", "regex": "destination"}, {"target_label": "oksg_source", "replacement": IDMeshIstio}, {"source_labels": []string{"source_canonical_service"}, "target_label": "client"}, {"source_labels": []string{"source_workload_namespace"}, "target_label": "client_namespace"}, {"source_labels": []string{"destination_canonical_service"}, "target_label": "server"}, {"source_labels": []string{"destination_service_namespace"}, "target_label": "server_namespace"}, {"source_labels": []string{"request_protocol"}, "target_label": "network_protocol_name"}, {"source_labels": []string{"response_code"}, "target_label": "http_response_status_code"}, {"source_labels": []string{"grpc_response_status"}, "target_label": "rpc_grpc_status_code"}, {"source_labels": []string{"connection_security_policy"}, "target_label": "mesh_security_policy"}, {"source_labels": []string{"source_principal"}, "target_label": "mesh_peer_source_principal"}, {"source_labels": []string{"destination_principal"}, "target_label": "mesh_peer_destination_principal"}, {"source_labels": []string{"response_flags"}, "target_label": "mesh_envoy_response_flags"}}}}}}
	out.AgentMetricReceivers = append(out.AgentMetricReceivers, "prometheus")
}

func contributeServicegraph(out *CollectorContribution, _ *profile.ClusterProfile) {
	out.GatewayConnectors["service_graph"] = map[string]any{"metrics_flush_interval": "15s", "latency_histogram_buckets": []string{"100ms", "250ms", "500ms", "1s", "5s"}, "dimensions": []string{"http.method"}, "store": map[string]any{"ttl": "30s", "max_items": 1000}}
	out.GatewayMetricReceivers = append(out.GatewayMetricReceivers, "service_graph")
	guard := `datapoint.attributes["oksg_source"] == nil`
	out.GatewaySourceStamps = append(out.GatewaySourceStamps,
		`set(datapoint.attributes["oksg_source"], "`+IDTracesServicegraph+`") where IsMatch(metric.name, "^traces_service_graph_") and `+guard,
		`set(datapoint.attributes["oksg_source"], "`+IDTracesServicegraph+`") where IsMatch(metric.name, "^traces\\.service\\.graph\\.") and `+guard,
	)
}

func defaultString(value, fallback string) string {
	if value == "" {
		return fallback
	}
	return value
}
