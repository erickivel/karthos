package profile

import (
	"time"

	"github.com/erickivel/oksg/pkg/core/entities"
)

type ClusterProfile struct {
	ClusterID    entities.EntityID     `json:"cluster_id"`
	DiscoveredAt time.Time             `json:"discovered_at"`
	Nodes        []entities.Node       `json:"nodes,omitempty"`
	Namespaces   []entities.Namespace  `json:"namespaces,omitempty"`
	Workloads    []entities.Workload   `json:"workloads,omitempty"`
	Services     []entities.Service    `json:"services,omitempty"`
	Sources      []ObservabilitySource `json:"sources,omitempty"`
}

type ObservabilitySource struct {
	Name    string `json:"name"`
	Type    string `json:"type"`
	Present bool   `json:"present"`
}
