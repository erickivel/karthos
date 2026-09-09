package ingest

import (
	"context"

	"github.com/erickivel/karthos/pkg/core/entities"
	"github.com/erickivel/karthos/pkg/core/metrics"
	"github.com/erickivel/karthos/pkg/core/schema"
	"github.com/erickivel/karthos/pkg/core/topology"
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
