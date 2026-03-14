package query

import (
	"context"
	"time"

	"github.com/erickivel/oksg/pkg/core/entities"
	"github.com/erickivel/oksg/pkg/core/metrics"
	"github.com/erickivel/oksg/pkg/core/topology"
)

type MetricQuery struct {
	EntityIDs  []entities.EntityID
	MetricName string
	From       time.Time
	To         time.Time
}

type EdgeQuery struct {
	Source entities.EntityID
	Target entities.EntityID
}

type GraphQuerier interface {
	GetLatestGraph(ctx context.Context) (*topology.Snapshot, error)
	QueryEdges(ctx context.Context, query EdgeQuery) ([]topology.Edge, error)
}

type MetricQuerier interface {
	QueryMetrics(ctx context.Context, query MetricQuery) ([]metrics.MetricSample, error)
}
