package ingest

import (
	"context"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/schema"
	"github.com/erickivel/oksg/pkg/core/topology"
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
// BuildNodes so BufferedTopologyIngestor can populate Graph.Nodes in a snapshot.
type NodeBuilder interface {
	BuildNodes(ctx context.Context, envelope schema.RawEnvelope) []topology.Node
}
