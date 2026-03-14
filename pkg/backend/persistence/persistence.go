package persistence

import (
	"context"

	"github.com/erickivel/oksg/pkg/backend/query"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/topology"
	"github.com/erickivel/oksg/pkg/discovery/profile"
)

type MetricStore interface {
	Write(ctx context.Context, samples []metrics.MetricSample) error
	Query(ctx context.Context, q query.MetricQuery) ([]metrics.MetricSample, error)
}

type GraphStore interface {
	PutSnapshot(ctx context.Context, snapshot topology.Snapshot) error
	GetLatest(ctx context.Context) (*topology.Snapshot, error)
	QueryEdges(ctx context.Context, q query.EdgeQuery) ([]topology.Edge, error)
}

type MetadataStore interface {
	PutDiscovery(ctx context.Context, snapshot profile.ClusterProfile) error
	GetDiscovery(ctx context.Context) (*profile.ClusterProfile, error)
}
