package profile

import (
	"time"

	"github.com/erickivel/oksg/pkg/core/entities"
)

type ClusterProfile struct {
	ClusterID            entities.EntityID        `json:"cluster_id"`
	ClusterName          string                   `json:"cluster_name"`
	DiscoveredAt         time.Time                `json:"discovered_at"`
	Nodes                []entities.Node          `json:"nodes,omitempty"`
	Namespaces           []entities.Namespace     `json:"namespaces,omitempty"`
	Workloads            []entities.Workload      `json:"workloads,omitempty"`
	HPAs                 []entities.HPA           `json:"hpas,omitempty"`
	ResourceQuotas       []entities.ResourceQuota `json:"resource_quotas,omitempty"`
	Pods                 []entities.Pod           `json:"pods,omitempty"`
	Services             []entities.Service       `json:"services,omitempty"`
	Ingresses            []entities.Ingress       `json:"ingresses,omitempty"`
	Sources              []ObservabilitySource    `json:"sources,omitempty"`
	CollectionPaths      []OTelCollectionPath     `json:"collection_paths,omitempty"`
	RecommendedReceivers []string                 `json:"recommended_receivers,omitempty"`
}

type PrometheusTarget struct {
	ServiceName string `json:"service_name"`
	Namespace   string `json:"namespace"`
	Port        int32  `json:"port"`
	Path        string `json:"path"`
	Reachable   bool   `json:"reachable"`
}

type OTLPSource struct {
	MetricsEnabled bool   `json:"metrics_enabled"`
	TracesEnabled  bool   `json:"traces_enabled"`
	GRPCEndpoint   string `json:"grpc_endpoint,omitempty"`
	HTTPEndpoint   string `json:"http_endpoint,omitempty"`
}

type MeshSource struct {
	Flavor             string   `json:"flavor"`
	Mode               string   `json:"mode"`
	InjectedNamespaces []string `json:"injected_namespaces,omitempty"`
	MetricsPort        int32    `json:"metrics_port,omitempty"`
	MetricsPath        string   `json:"metrics_path,omitempty"`
}

type ObservabilitySource struct {
	Name              string             `json:"name"`
	Type              string             `json:"type"`
	Present           bool               `json:"present"`
	PrometheusTargets []PrometheusTarget `json:"prometheus_targets,omitempty"`
	OTLP              *OTLPSource        `json:"otlp,omitempty"`
	Mesh              *MeshSource        `json:"mesh,omitempty"`
}

type OTelCollectionPath struct {
	ReceiverType string   `json:"receiver_type"`
	Enabled      bool     `json:"enabled"`
	Targets      []string `json:"targets,omitempty"`
	SignalTypes  []string `json:"signal_types,omitempty"`
}
