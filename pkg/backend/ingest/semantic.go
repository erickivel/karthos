package ingest

import (
	"context"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/topology"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

type Normalizer interface {
	Normalize(ctx context.Context, envelope schema.RawEnvelope) ([]metrics.MetricSample, error)
}

type EntityResolver interface {
	Resolve(ctx context.Context, resource map[string]string) (entities.EntityID, error)
}

type TopologyBuilder interface {
	Build(ctx context.Context, envelope schema.RawEnvelope) ([]topology.Edge, error)
}

// NodeBuilder is an optional extension of TopologyBuilder. Implementations that
// know the EntityKind for each entity they reference can additionally implement
// BuildNodes so the TopologyIngestor can populate Graph.Nodes in the snapshot.
type NodeBuilder interface {
	BuildNodes(ctx context.Context, envelope schema.RawEnvelope) []topology.Node
}

// StructuralTopologyBuilder builds topology edges and nodes from Kubernetes
// structural data (discovery profiles), as opposed to OTLP telemetry data.
type StructuralTopologyBuilder interface {
	BuildStructural(ctx context.Context, prof *profile.ClusterProfile) ([]topology.Edge, []topology.Node, error)
}
