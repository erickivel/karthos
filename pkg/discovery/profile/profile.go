package profile

import (
	"time"

	"github.com/erickivel/oksg/pkg/core/entities"
)

type ClusterProfile struct {
	ClusterID            entities.EntityID     `json:"cluster_id"`
	ClusterName          string                `json:"cluster_name"`
	DiscoveredAt         time.Time             `json:"discovered_at"`
	Nodes                []entities.Node       `json:"nodes,omitempty"`
	Namespaces           []entities.Namespace  `json:"namespaces,omitempty"`
	Workloads            []entities.Workload   `json:"workloads,omitempty"`
	Services             []entities.Service    `json:"services,omitempty"`
	Sources              []ObservabilitySource `json:"sources,omitempty"`
	CollectionPaths      []OTelCollectionPath  `json:"collection_paths,omitempty"`
	RecommendedReceivers []string              `json:"recommended_receivers,omitempty"`
}

type PrometheusTarget struct {
	ServiceName string `json:"service_name"`
	Namespace   string `json:"namespace"`
	Port        int32  `json:"port"`
	Path        string `json:"path"`
	Reachable   bool   `json:"reachable"`
}

type ObservabilitySource struct {
	Name              string             `json:"name"`
	Type              string             `json:"type"`
	Present           bool               `json:"present"`
	PrometheusTargets []PrometheusTarget `json:"prometheus_targets,omitempty"`
}

type OTelCollectionPath struct {
	ReceiverType string   `json:"receiver_type"`
	Enabled      bool     `json:"enabled"`
	Targets      []string `json:"targets,omitempty"`
}
